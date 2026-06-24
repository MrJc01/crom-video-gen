package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"crom-video-gen/internal/execs"

	"cromedia/core"
	"cromedia/core/demux"
	"cromedia/core/mux"
)

// Narrator define a abstração para qualquer serviço de conversão texto-para-fala
type Narrator interface {
	// Narrate gera o áudio, salva-o no caminho especificado e retorna a duração dele em segundos
	Narrate(texto string, voz string, rate string, pitch string, volume string, outputPath string) (float64, error)
}

// MockNarrator é um adaptador offline para desenvolvimento/testes locais.
// Ele gera um arquivo de áudio silenciado com duração proporcional ao número de palavras.
type MockNarrator struct{}

// NewMockNarrator instancia um MockNarrator
func NewMockNarrator() *MockNarrator {
	return &MockNarrator{}
}

// Narrate gera um áudio de tom senoidal de teste no outputPath usando codificação nativa WAV
func (mn *MockNarrator) Narrate(texto string, voz string, rate string, pitch string, volume string, outputPath string) (float64, error) {
	words := len(strings.Fields(texto))
	// Simulação: 3 palavras por segundo, mínimo de 2 segundos.
	duration := float64(words) / 2.5
	if duration < 2.0 {
		duration = 2.0
	}

	// Cria o arquivo de saída
	file, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("falha ao criar arquivo de áudio de teste: %w", err)
	}
	defer file.Close()

	sampleRate := 44100
	track := core.Track{
		ID:        1,
		Type:      core.TrackTypeAudio,
		Timescale: uint32(sampleRate),
	}

	wavMuxer := mux.NewWAVMuxer(file)
	if err := wavMuxer.WriteHeader([]core.Track{track}); err != nil {
		return 0, fmt.Errorf("falha ao escrever cabeçalho WAV: %w", err)
	}

	// Gera tom senoidal de 440 Hz
	freq := 440.0
	volFactor := 0.15
	totalSamples := int(duration * float64(sampleRate))
	pcmData := make([]byte, totalSamples*4) // Stereo, 16-bit (4 bytes por sample)

	for i := 0; i < totalSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2.0*math.Pi*freq*t) * volFactor
		intVal := int16(val * 32767.0)

		// Canal Esquerdo
		binary.LittleEndian.PutUint16(pcmData[i*4:i*4+2], uint16(intVal))
		// Canal Direito
		binary.LittleEndian.PutUint16(pcmData[i*4+2:i*4+4], uint16(intVal))
	}

	packet := &core.Packet{Data: pcmData}
	if err := wavMuxer.WritePacket(packet); err != nil {
		return 0, fmt.Errorf("falha ao escrever dados de áudio PCM: %w", err)
	}

	if err := wavMuxer.WriteTrailer(); err != nil {
		return 0, fmt.Errorf("falha ao finalizar arquivo WAV: %w", err)
	}

	return duration, nil
}

// EdgeTTSNarrator é um adaptador para o serviço de voz em nuvem da Microsoft Edge (edge-tts)
type EdgeTTSNarrator struct{}

// NewEdgeTTSNarrator instancia um EdgeTTSNarrator
func NewEdgeTTSNarrator() *EdgeTTSNarrator {
	return &EdgeTTSNarrator{}
}

// Narrate gera o áudio usando edge-tts CLI e retorna a duração real extraída via ffprobe
func (en *EdgeTTSNarrator) Narrate(texto string, voz string, rate string, pitch string, volume string, outputPath string) (float64, error) {
	edgeTTSPath := execs.ResolveEdgeTTSPath()

	// Mapeia vozes antigas ou vazias para vozes válidas do Edge-TTS
	edgeVoice := voz
	if edgeVoice == "" || !strings.Contains(edgeVoice, "-") {
		lowerVoice := strings.ToLower(edgeVoice)
		if strings.Contains(lowerVoice, "female") {
			edgeVoice = "pt-BR-FranciscaNeural"
		} else if strings.Contains(lowerVoice, "male") || strings.Contains(lowerVoice, "premium") {
			edgeVoice = "pt-BR-AntonioNeural"
		} else {
			edgeVoice = "pt-BR-FranciscaNeural" // Fallback padrão
		}
	}

	args := []string{"-t", texto}
	if edgeVoice != "" {
		args = append(args, "-v", edgeVoice)
	}
	if rate != "" {
		args = append(args, "--rate="+rate)
	}
	if pitch != "" {
		args = append(args, "--pitch="+pitch)
	}
	if volume != "" {
		args = append(args, "--volume="+volume)
	}
	args = append(args, "--write-media", outputPath)

	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		cmd := exec.CommandContext(ctx, edgeTTSPath, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("timeout ao gerar áudio com edge-tts (25s excedido)")
		} else {
			err = fmt.Errorf("erro: %w (stderr: %s)", err, stderr.String())
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return 0, fmt.Errorf("falha ao gerar áudio com edge-tts (%s) após 3 tentativas: %w", edgeTTSPath, err)
	}

	// Extrai a duração real do áudio gerado
	duration, err := GetAudioDuration(outputPath)
	if err != nil {
		return 0, fmt.Errorf("falha ao ler duração do áudio gerado pelo edge-tts: %w", err)
	}

	return duration, nil
}

// GetAudioDuration extrai a duração de um arquivo de áudio/vídeo em segundos usando os demuxers do CroMedia
func GetAudioDuration(filePath string) (float64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	format, err := demux.SniffFormat(file)
	if err != nil {
		return 0, fmt.Errorf("falha ao identificar formato do arquivo %s: %w", filePath, err)
	}

	dm, err := demux.NewDemuxerFromFormat(format, file)
	if err != nil {
		return 0, fmt.Errorf("falha ao criar demuxer para formato %s: %w", format, err)
	}
	defer dm.Close()

	tracks, err := dm.Probe()
	if err != nil {
		return 0, fmt.Errorf("falha ao ler metadados do arquivo %s: %w", filePath, err)
	}

	var maxDuration float64
	for _, track := range tracks {
		if track.Timescale > 0 {
			dur := float64(track.Duration) / float64(track.Timescale)
			if dur > maxDuration {
				maxDuration = dur
			}
		}
	}

	if maxDuration == 0 {
		return 0, fmt.Errorf("duração inválida ou zero encontrada no arquivo %s", filePath)
	}

	return maxDuration, nil
}
