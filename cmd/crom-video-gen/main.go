package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"crom-video-gen/pkg/generator"
)

func main() {
	// Definição das flags do terminal
	configPath := flag.String("config", "json_inicial", "Caminho para o arquivo JSON de configuração")
	outputPath := flag.String("output", "output.mp4", "Caminho do arquivo de vídeo gerado")
	logFormat := flag.String("log-format", "text", "Formato de logs: 'text' ou 'json'")
	ttsProvider := flag.String("tts-provider", "edge-tts", "Provedor de narração TTS: 'mock' ou 'edge-tts'")
	validateOnly := flag.Bool("validate-only", false, "Apenas valida a estrutura do JSON e a presença física dos ativos, sem renderizar")
	verbose := flag.Bool("verbose", false, "Ativa modo log verboso (debug)")

	flag.Parse()

	// Configuração do Logger Estruturado (slog)
	var logLevel slog.Level = slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}

	var handler slog.Handler
	switch strings.ToLower(*logFormat) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Captura de sinais do sistema operacional para encerramento gracioso
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Warn("Sinal de encerramento recebido, cancelando execução...", "sinal", sig.String())
		cancel()
	}()

	// Executa a pipeline
	err := generator.GenerateVideo(ctx, logger, *configPath, *outputPath, *ttsProvider, *validateOnly)
	if err != nil {
		logger.Error("Execução finalizada com erro", "erro", err.Error())

		// Determina o exit code de acordo com o tipo de erro
		errStr := err.Error()
		if strings.Contains(errStr, "parsing de configuração") || strings.Contains(errStr, "inválido") {
			os.Exit(2) // Erro de validação de dados / config
		} else if strings.Contains(errStr, "arquivo não encontrado") || strings.Contains(errStr, "trilha sonora inválida") || strings.Contains(errStr, "ativo inválido") {
			os.Exit(3) // Erro de validação física de ativos
		} else if strings.Contains(errStr, "renderizar") || strings.Contains(errStr, "FFmpeg") || strings.Contains(errStr, "mixar") {
			os.Exit(4) // Erro de execução do FFmpeg
		}

		os.Exit(1) // Outros erros internos
	}

	logger.Info("Vídeo gerado e pronto com sucesso!")
	os.Exit(0)
}
