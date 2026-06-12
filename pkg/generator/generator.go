package generator

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"crom-video-gen/internal/assets"
	"crom-video-gen/internal/execs"
	"crom-video-gen/pkg/render"
	"crom-video-gen/pkg/tts"
	"crom-video-gen/pkg/types"
)

// GenerateVideo é o orquestrador principal que executa a validação e renderização completa do vídeo
func GenerateVideo(ctx context.Context, logger *slog.Logger, configPath string, outputPath string, ttsProvider string, validateOnly bool) error {
	logger.Info("Iniciando processo de geração de vídeo", "config", configPath, "output", outputPath)

	// Resolve o outputPath para absoluto para garantir que a escrita final funcione em qualquer subpasta
	if absOut, err := filepath.Abs(outputPath); err == nil {
		outputPath = absOut
	}

	// 1. Parsing do arquivo de configuração JSON
	config, err := types.ParseConfig(configPath)
	if err != nil {
		return fmt.Errorf("erro no parsing de configuração: %w", err)
	}
	logger.Info("Configuração lida e validada logicamente com sucesso", "titulo", config.Projeto.Titulo)

	// 2. Validação física dos ativos (imagens, vídeos, trilha sonora)
	root := execs.FindProjectRoot()
	am := assets.NewAssetManager(root)

	// Valida trilha sonora
	if err := am.ValidateFile(config.Projeto.TrilhaSonora.Arquivo, []string{".mp3", ".wav"}); err != nil {
		return fmt.Errorf("trilha sonora inválida: %w", err)
	}

	// Valida cada cena e seus ativos, convertendo-os para absoluto
	for i, cena := range config.Projeto.Cenas {
		for key, ativo := range cena.Ativos {
			var allowedExts []string
			if ativo.Tipo == "imagem" {
				allowedExts = []string{".jpg", ".jpeg", ".png", ".webp"}
			} else {
				allowedExts = []string{".mp4", ".mkv", ".avi", ".mov"}
			}

			if err := am.ValidateFile(ativo.Caminho, allowedExts); err != nil {
				return fmt.Errorf("ativo '%s' inválido na cena %d: %w", key, cena.ID, err)
			}

			absPath, err := am.SanitizePath(ativo.Caminho)
			if err != nil {
				return err
			}
			config.Projeto.Cenas[i].Ativos[key] = types.Ativo{
				Tipo:    ativo.Tipo,
				Caminho: absPath,
			}
		}
	}
	logger.Info("Todos os ativos físicos foram localizados e validados com sucesso")

	// Se o usuário solicitou apenas validação, encerra por aqui
	if validateOnly {
		logger.Info("Modo de validação ativa: Processo concluído com sucesso (sem renderização)")
		return nil
	}

	// 3. Inicializa o diretório temporário para renderização
	tempDir, err := am.CreateTempDir()
	if err != nil {
		return err
	}
	// Garante a limpeza da pasta temporária no final
	defer func() {
		logger.Debug("Limpando diretório temporário de trabalho", "dir", tempDir)
		if err := am.Cleanup(); err != nil {
			logger.Error("Erro ao limpar diretório temporário", "erro", err)
		}
	}()
	logger.Debug("Diretório temporário criado para renderização", "dir", tempDir)

	// 4. Inicializa os motores de TTS
	mockNarrator := tts.NewMockNarrator()
	edgeTTSNarrator := tts.NewEdgeTTSNarrator()

	// Valida o ttsProvider global recebido pela flag
	globalProv := strings.ToLower(ttsProvider)
	if globalProv != "" && globalProv != "mock" && globalProv != "edge-tts" {
		return fmt.Errorf("provedor de TTS global '%s' não suportado (apenas 'mock' e 'edge-tts' são permitidos)", ttsProvider)
	}

	var intermediateSceneFiles []string

	// 5. Renderiza cada cena individualmente
	for i, cena := range config.Projeto.Cenas {
		logger.Info("Processando cena", "progresso_etapa", fmt.Sprintf("%d/%d", i+1, len(config.Projeto.Cenas)), "cena_id", cena.ID, "template", cena.Template.ID)

		// Escolhe o provedor para a cena (específico da cena se fornecido, senão o global)
		prov := cena.Narracao.Provedor
		if prov == "" {
			prov = ttsProvider
		}

		var narrator tts.Narrator
		switch strings.ToLower(prov) {
		case "edge-tts":
			narrator = edgeTTSNarrator
		case "mock", "":
			narrator = mockNarrator
		default:
			return fmt.Errorf("provedor de TTS '%s' não suportado na cena %d", prov, cena.ID)
		}

		// Caminho temporário para salvar o áudio de narração da cena
		audioOut := filepath.Join(tempDir, fmt.Sprintf("cena_%d_narration.mp3", cena.ID))
		duration, err := narrator.Narrate(cena.Narracao.Texto, cena.Narracao.Voz, cena.Narracao.Rate, cena.Narracao.Pitch, cena.Narracao.Volume, audioOut)
		if err != nil {
			return fmt.Errorf("falha ao gerar áudio de narração para cena %d: %w", cena.ID, err)
		}
		logger.Debug("Áudio de narração gerado", "cena_id", cena.ID, "duracao_segundos", duration)

		// Verifica se o arquivo de áudio foi realmente gerado e extrai a duração com ffprobe
		probedDuration, err := tts.GetAudioDuration(audioOut)
		if err != nil {
			return fmt.Errorf("falha ao inspecionar áudio gerado na cena %d: %w", cena.ID, err)
		}

		// Caminho temporário para salvar o vídeo individual renderizado da cena
		videoOut := filepath.Join(tempDir, fmt.Sprintf("cena_%d_rendered.mp4", cena.ID))
		err = render.RenderScene(ctx, logger, &cena, &config.Projeto.ConfiguracoesGlobais, audioOut, probedDuration, videoOut)
		if err != nil {
			return fmt.Errorf("falha ao renderizar cena %d: %w", cena.ID, err)
		}

		intermediateSceneFiles = append(intermediateSceneFiles, videoOut)
		logger.Info("Cena renderizada com sucesso", "cena_id", cena.ID, "duracao", probedDuration)
	}

	// 6. Concatena os clipes de vídeo das cenas e adiciona a trilha sonora
	logger.Info("Concatenando cenas e adicionando trilha sonora final...")
	soundtrackPath, err := am.SanitizePath(config.Projeto.TrilhaSonora.Arquivo)
	if err != nil {
		return fmt.Errorf("falha ao processar caminho da trilha sonora: %w", err)
	}

	err = render.ConcatScenes(
		ctx,
		logger,
		intermediateSceneFiles,
		soundtrackPath,
		config.Projeto.TrilhaSonora.Volume,
		&config.Projeto.ConfiguracoesGlobais.Audio,
		tempDir,
		outputPath,
	)
	if err != nil {
		return fmt.Errorf("falha na finalização e mixagem de vídeo: %w", err)
	}

	logger.Info("Processo de geração de vídeo finalizado com sucesso!", "arquivo_gerado", outputPath)
	return nil
}
