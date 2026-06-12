package render

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"crom-video-gen/internal/execs"
	"crom-video-gen/pkg/types"

	"github.com/chromedp/chromedp"
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

	projectRoot := execs.FindProjectRoot()

	// Inicia local HTTP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	localAddr := listener.Addr().String()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(projectRoot)))
	httpServer := &http.Server{Handler: mux}

	go func() {
		_ = httpServer.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelShutdown()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	// Inicia chromedp allocator e context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox,
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx)
	defer chromeCancel()

	// Pre-boot browser
	_ = chromedp.Run(chromeCtx, chromedp.Navigate("about:blank"))

	// Renderiza a cena
	err = RenderScene(context.Background(), logger, chromeCtx, localAddr, projectRoot, cena, globalConf, testAudio, 3.0, outputScene)
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
