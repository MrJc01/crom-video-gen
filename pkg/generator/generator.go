package generator

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"crom-video-gen/internal/assets"
	"crom-video-gen/internal/execs"
	"crom-video-gen/pkg/render"
	"crom-video-gen/pkg/tts"
	"crom-video-gen/pkg/types"

	"github.com/chromedp/chromedp"
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

	// 4. Inicializa o servidor HTTP local compartilhado para servir templates/ativos
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("falha ao iniciar listener HTTP local compartilhado: %w", err)
	}
	localAddr := listener.Addr().String()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(root)))

	httpServer := &http.Server{
		Handler: mux,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("Erro no servidor HTTP local compartilhado", "erro", err)
		}
	}()
	defer func() {
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancelShutdown()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	logger.Debug("Servidor HTTP local compartilhado ativo", "addr", localAddr)

	// 5. Inicializa uma única instância do Chrome compartilhada
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.NoSandbox, // Compatibilidade com Linux e ambientes dockerizados
		chromedp.Flag("hide-scrollbars", true),
		chromedp.WindowSize(1920, 1080),
		// Performance/Headless flags:
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx)
	defer chromeCancel()

	// Inicializa a instância do navegador executando uma navegação em branco
	if err := chromedp.Run(chromeCtx, chromedp.Navigate("about:blank")); err != nil {
		return fmt.Errorf("falha ao iniciar browser compartilhado: %w", err)
	}
	logger.Debug("Instância compartilhada do Chrome iniciada com sucesso")

	// 6. Inicializa os motores de TTS
	mockNarrator := tts.NewMockNarrator()
	edgeTTSNarrator := tts.NewEdgeTTSNarrator()

	// Valida o ttsProvider global recebido pela flag
	globalProv := strings.ToLower(ttsProvider)
	if globalProv != "" && globalProv != "mock" && globalProv != "edge-tts" {
		return fmt.Errorf("provedor de TTS global '%s' não suportado (apenas 'mock' e 'edge-tts' são permitidos)", ttsProvider)
	}

	// Estrutura para guardar a duração real de cada cena após TTS concorrente
	sceneDurations := make([]float64, len(config.Projeto.Cenas))
	sceneAudioFiles := make([]string, len(config.Projeto.Cenas))

	logger.Info("Iniciando geração concorrente de áudios TTS...")
	
	var ttsWg sync.WaitGroup
	ttsErrChan := make(chan error, len(config.Projeto.Cenas))

	for i, cena := range config.Projeto.Cenas {
		ttsWg.Add(1)
		go func(idx int, c types.Cena) {
			defer ttsWg.Done()
			
			prov := c.Narracao.Provedor
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
				ttsErrChan <- fmt.Errorf("provedor de TTS '%s' não suportado na cena %d", prov, c.ID)
				return
			}

			audioOut := filepath.Join(tempDir, fmt.Sprintf("cena_%d_narration.mp3", c.ID))
			_, err := narrator.Narrate(c.Narracao.Texto, c.Narracao.Voz, c.Narracao.Rate, c.Narracao.Pitch, c.Narracao.Volume, audioOut)
			if err != nil {
				ttsErrChan <- fmt.Errorf("falha ao gerar áudio de narração para cena %d: %w", c.ID, err)
				return
			}

			probedDuration, err := tts.GetAudioDuration(audioOut)
			if err != nil {
				ttsErrChan <- fmt.Errorf("falha ao inspecionar áudio gerado na cena %d: %w", c.ID, err)
				return
			}

			sceneDurations[idx] = probedDuration
			sceneAudioFiles[idx] = audioOut
			logger.Debug("Áudio de narração gerado (concorrente)", "cena_id", c.ID, "duracao", probedDuration)
		}(i, cena)
	}

	ttsWg.Wait()
	close(ttsErrChan)

	// Verifica se ocorreu algum erro durante a geração paralela de TTS
	for err := range ttsErrChan {
		if err != nil {
			return err
		}
	}

	logger.Info("Todos os áudios TTS foram gerados com sucesso")

	// Limita concorrência de renderização
	maxWorkers := runtime.NumCPU() / 2
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if maxWorkers > 4 {
		maxWorkers = 4 // limite prudente de RAM/Chrome tabs
	}
	logger.Info("Iniciando renderização paralela de cenas", "workers", maxWorkers)

	type Result struct {
		Index     int
		VideoPath string
		Err       error
	}

	resultsChan := make(chan Result, len(config.Projeto.Cenas))
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	// 7. Renderiza cada cena concorrentemente
	for i, cena := range config.Projeto.Cenas {
		wg.Add(1)
		go func(idx int, c types.Cena, audioPath string, duration float64) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			logger.Info("Processando cena (concorrente)", "progresso_etapa", fmt.Sprintf("%d/%d", idx+1, len(config.Projeto.Cenas)), "cena_id", c.ID, "template", c.Template.ID)

			// Caminho temporário para salvar o vídeo individual renderizado da cena
			videoOut := filepath.Join(tempDir, fmt.Sprintf("cena_%d_rendered.mp4", c.ID))
			err = render.RenderScene(ctx, logger, chromeCtx, localAddr, root, &c, &config.Projeto.ConfiguracoesGlobais, audioPath, duration, videoOut)
			if err != nil {
				resultsChan <- Result{Index: idx, Err: fmt.Errorf("falha ao renderizar cena %d: %w", c.ID, err)}
				return
			}

			logger.Info("Cena renderizada com sucesso", "cena_id", c.ID, "duracao", duration)
			resultsChan <- Result{Index: idx, VideoPath: videoOut}
		}(i, cena, sceneAudioFiles[i], sceneDurations[i])
	}

	wg.Wait()
	close(resultsChan)

	intermediateSceneFiles := make([]string, len(config.Projeto.Cenas))
	for res := range resultsChan {
		if res.Err != nil {
			return res.Err
		}
		intermediateSceneFiles[res.Index] = res.VideoPath
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
