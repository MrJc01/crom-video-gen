package tts

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"crom-video-gen/internal/execs"
)

func TestMockNarrator_And_GetAudioDuration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tts_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	mn := NewMockNarrator()
	outputPath := filepath.Join(tempDir, "narration.mp3")

	texto := "Esta é uma frase de teste com oito palavras inteiras."

	duration, err := mn.Narrate(texto, "dummy_voice", "", "", "", outputPath)
	if err != nil {
		t.Fatalf("Erro ao narrar: %v", err)
	}

	if duration <= 0 {
		t.Errorf("Duração deve ser maior que zero, obtido %f", duration)
	}

	// Verifica se arquivo existe
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Arquivo de áudio não foi criado: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Arquivo de áudio está vazio")
	}

	// Testa leitura com ffprobe
	probedDuration, err := GetAudioDuration(outputPath)
	if err != nil {
		t.Fatalf("Erro ao ler duração com ffprobe: %v", err)
	}

	// A duração lida pelo ffprobe deve ser extremamente próxima da gerada
	diff := math.Abs(duration - probedDuration)
	if diff > 0.1 {
		t.Errorf("Duração retornada pelo Narrate (%f) difere muito da lida pelo ffprobe (%f). Diff: %f", duration, probedDuration, diff)
	}
}

func TestEdgeTTSNarrator(t *testing.T) {
	path := execs.ResolveEdgeTTSPath()
	if path == "edge-tts" {
		_, err := exec.LookPath("edge-tts")
		if err != nil {
			t.Skip("edge-tts não instalado no PATH do sistema, pulando teste")
		}
	}

	tempDir, err := os.MkdirTemp("", "tts_edge_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	en := NewEdgeTTSNarrator()
	outputPath := filepath.Join(tempDir, "narration_edge.mp3")

	texto := "Teste do motor Edge-TTS em Go."
	duration, err := en.Narrate(texto, "pt-BR-FranciscaNeural", "+10%", "-5Hz", "+0%", outputPath)
	if err != nil {
		t.Fatalf("Erro ao narrar com Edge-TTS: %v", err)
	}

	if duration <= 0 {
		t.Errorf("Duração deve ser maior que zero, obtido %f", duration)
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Arquivo de áudio não foi criado pelo Edge-TTS: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Arquivo de áudio gerado pelo Edge-TTS está vazio")
	}

	probedDuration, err := GetAudioDuration(outputPath)
	if err != nil {
		t.Fatalf("Erro ao ler duração com ffprobe: %v", err)
	}

	diff := math.Abs(duration - probedDuration)
	if diff > 0.1 {
		t.Errorf("Duração retornada pelo Narrate (%f) difere do ffprobe (%f). Diff: %f", duration, probedDuration, diff)
	}
}
