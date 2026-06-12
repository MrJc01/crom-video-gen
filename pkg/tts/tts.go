package tts

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"crom-video-gen/internal/execs"
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

// Narrate gera um áudio silenciado no outputPath usando FFmpeg e retorna a duração simulada
func (mn *MockNarrator) Narrate(texto string, voz string, rate string, pitch string, volume string, outputPath string) (float64, error) {
	words := len(strings.Fields(texto))
	// Simulação: 3 palavras por segundo, mínimo de 2 segundos.
	duration := float64(words) / 2.5
	if duration < 2.0 {
		duration = 2.0
	}

	// Executa ffmpeg para gerar áudio silenciado com a duração correspondente
	// Usamos codec libmp3lame ou aac de acordo com a extensão
	codec := "libmp3lame"
	if strings.HasSuffix(strings.ToLower(outputPath), ".aac") {
		codec = "aac"
	}

	ffmpegPath := execs.ResolveFFmpegPath()
	cmd := exec.Command(ffmpegPath, "-y",
		"-f", "lavfi",
		"-i", "sine=frequency=440:sample_rate=48000",
		"-af", "volume=0.15",
		"-c:a", codec,
		"-t", fmt.Sprintf("%.3f", duration),
		outputPath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("falha ao gerar áudio de teste com ffmpeg (%s): %w (detalhes: %s)", ffmpegPath, err, stderr.String())
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

// GetAudioDuration invoca o ffprobe e extrai a duração do arquivo de áudio em segundos
func GetAudioDuration(filePath string) (float64, error) {
	ffprobePath := execs.ResolveFFprobePath()
	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return 0, fmt.Errorf("falha ao ler duração com ffprobe (%s): %w (detalhes: %s)", ffprobePath, err, stderr.String())
	}


	durationStr := strings.TrimSpace(stdout.String())
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0, fmt.Errorf("falha ao interpretar duração '%s': %w", durationStr, err)
	}

	return duration, nil
}
