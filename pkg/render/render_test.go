package render

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"crom-video-gen/internal/execs"
	"crom-video-gen/pkg/types"
)

func generateTestImage(t *testing.T, ffmpegPath, outputPath string) {
	cmd := exec.Command(ffmpegPath, "-y",
		"-f", "lavfi",
		"-i", "color=c=blue:s=640x480:d=1",
		"-vframes", "1",
		outputPath,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Erro ao gerar imagem de teste com ffmpeg: %v", err)
	}
}

func generateTestAudio(t *testing.T, ffmpegPath, outputPath string) {
	cmd := exec.Command(ffmpegPath, "-y",
		"-f", "lavfi",
		"-i", "anullsrc=r=48000:cl=stereo",
		"-c:a", "aac",
		"-t", "3",
		outputPath,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Erro ao gerar áudio de teste com ffmpeg: %v", err)
	}
}

func TestRenderScene_IntroBranding(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "render_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ffmpegPath := execs.ResolveFFmpegPath()

	// Gera ativos fictícios para o teste
	testImg := filepath.Join(tempDir, "imagem.jpg")
	testAudio := filepath.Join(tempDir, "narracao.aac")
	outputScene := filepath.Join(tempDir, "cena_1.mp4")

	generateTestImage(t, ffmpegPath, testImg)
	generateTestAudio(t, ffmpegPath, testAudio)

	globalConf := &types.GlobalConfig{
		Resolucao:    "640x480", // Resolução reduzida para o teste rodar super rápido
		FPS:          30,
		FormatoSaida: "mp4",
		Audio: types.AudioConfig{
			SampleRate: 48000,
			Codec:      "aac",
			Canais:     2,
		},
	}

	cena := &types.Cena{
		ID: 1,
		Template: types.Template{
			ID: "intro_branding",
			Parametros: map[string]interface{}{
				"overlay_opacity": 0.5,
			},
		},
		Ativos: map[string]types.Ativo{
			"media0": {
				Tipo:    "imagem",
				Caminho: testImg,
			},
		},
		Narracao: types.Narracao{
			Texto: "Bem vindo a documentacao técnica do ecossistema crom.",
		},
	}

	// Criamos um logger básico descartável para o teste
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Renderiza a cena
	err = RenderScene(context.Background(), logger, cena, globalConf, testAudio, 3.0, outputScene)
	if err != nil {
		t.Fatalf("Erro ao renderizar cena: %v", err)
	}

	// Verifica se arquivo existe e possui conteúdo
	info, err := os.Stat(outputScene)
	if err != nil {
		t.Fatalf("Arquivo de cena renderizado não existe: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("Arquivo de cena renderizado está com tamanho zero")
	}
}
