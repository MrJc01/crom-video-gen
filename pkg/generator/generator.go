package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
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

	"cromedia/core"
	"cromedia/core/demux"
	coreImage "cromedia/core/image"
	"cromedia/core/mux"

	"github.com/chromedp/chromedp"
)

// GenerateVideo é o orquestrador principal que executa a validação e renderização completa do vídeo
func GenerateVideo(ctx context.Context, logger *slog.Logger, configPath string, outputPath string, ttsProvider string, validateOnly bool, concurrency int) error {
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
	if config.Projeto.TrilhaSonora.Arquivo != "" {
		if err := am.ValidateFile(config.Projeto.TrilhaSonora.Arquivo, []string{".mp3", ".wav"}); err != nil {
			return fmt.Errorf("trilha sonora inválida: %w", err)
		}
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

	// 2.5. Otimiza os ativos de vídeo (keyint=1, scale, sem áudio) para busca instantânea no Chrome
	if err := OptimizeVideoAssets(ctx, logger, config, root); err != nil {
		return fmt.Errorf("falha ao otimizar ativos de vídeo: %w", err)
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
	fileServer := http.FileServer(http.Dir(root))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if ext == ".ttf" || ext == ".css" || ext == ".js" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" || ext == ".mp3" || ext == ".mp4" {
			w.Header().Set("Cache-Control", "public, max-age=31536000")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=60")
		}
		fileServer.ServeHTTP(w, r)
	})

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
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", "new"), // Usa o novo modo headless do Chrome (suporta aceleração por GPU)
		chromedp.NoSandbox,                // Compatibilidade com Linux e ambientes dockerizados
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
	}

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
	maxWorkers := concurrency
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU() / 2
		if maxWorkers < 1 {
			maxWorkers = 1
		}
		if maxWorkers > 4 {
			maxWorkers = 4 // limite prudente de RAM/Chrome tabs
		}
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
	var soundtrackPath string
	if config.Projeto.TrilhaSonora.Arquivo != "" {
		var err error
		soundtrackPath, err = am.SanitizePath(config.Projeto.TrilhaSonora.Arquivo)
		if err != nil {
			return fmt.Errorf("falha ao processar caminho da trilha sonora: %w", err)
		}
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

// OptimizeVideoAssets pré-processa os vídeos de fundo de todas as cenas.
// Ele redimensiona para a resolução do projeto, stripa o áudio, e força keyframe interval = 1.
// O arquivo resultante é salvo em assets/video/.cache/ para reuso rápido.
func OptimizeVideoAssets(ctx context.Context, logger *slog.Logger, config *types.ConfigInput, projectRoot string) error {
	w, h, err := config.Projeto.ConfiguracoesGlobais.GetResolutionWidthAndHeight()
	if err != nil {
		return fmt.Errorf("resolução global inválida: %w", err)
	}

	cacheDir := filepath.Join(projectRoot, "assets", "video", ".cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("falha ao criar pasta de cache de vídeos: %w", err)
	}

	// Mapeia vídeos processados para evitar transcodificar o mesmo vídeo duas vezes no mesmo projeto
	processed := make(map[string]string)

	for i, cena := range config.Projeto.Cenas {
		for key, ativo := range cena.Ativos {
			if ativo.Tipo != "video" {
				continue
			}

			originalPath := ativo.Caminho
			if cachedPath, ok := processed[originalPath]; ok {
				config.Projeto.Cenas[i].Ativos[key] = types.Ativo{
					Tipo:    "video",
					Caminho: cachedPath,
				}
				continue
			}

			info, err := os.Stat(originalPath)
			if err != nil {
				return fmt.Errorf("falha ao ler estatísticas do vídeo %s: %w", originalPath, err)
			}

			// Gera uma hash única com base nas propriedades do vídeo original para invalidar cache
			hashInput := fmt.Sprintf("%s_%d_%d_%dx%d", originalPath, info.Size(), info.ModTime().UnixNano(), w, h)
			hashBytes := sha256.Sum256([]byte(hashInput))
			hashStr := hex.EncodeToString(hashBytes[:8]) // 16 caracteres de hash

			baseName := filepath.Base(originalPath)
			ext := filepath.Ext(originalPath)
			cleanName := strings.TrimSuffix(baseName, ext)

			// Nome seguro do cache
			cacheFileName := fmt.Sprintf("%s_opt_%s.mp4", cleanName, hashStr)
			cacheFilePath := filepath.Join(cacheDir, cacheFileName)

			// Verifica se já existe no cache do disco
			if _, err := os.Stat(cacheFilePath); err == nil {
				logger.Info("Usando vídeo otimizado do cache", "original", baseName, "cache", cacheFileName)
				processed[originalPath] = cacheFilePath
				config.Projeto.Cenas[i].Ativos[key] = types.Ativo{
					Tipo:    "video",
					Caminho: cacheFilePath,
				}
				continue
			}

			// Transcoda o vídeo usando CroMedia nativo:
			// - keyint=1: garante que todo frame é I-frame (keyframe)
			// - scale=W:H: força redimensionamento do canvas
			// - an: remove áudio para economizar processamento do browser
			logger.Info("Transcodificando vídeo em background para keyframe-only (isso rodará apenas uma vez por vídeo)", "original", baseName, "saida", cacheFileName)

			startTime := time.Now()
			err = OptimizeVideoAssetNative(originalPath, cacheFilePath, w, h)
			if err != nil {
				return fmt.Errorf("CroMedia falhou ao transcodificar %s: %w", originalPath, err)
			}
			logger.Info("Vídeo transcodificado com sucesso", "tempo", time.Since(startTime), "arquivo", cacheFileName)

			processed[originalPath] = cacheFilePath
			config.Projeto.Cenas[i].Ativos[key] = types.Ativo{
				Tipo:    "video",
				Caminho: cacheFilePath,
			}
		}
	}
	return nil
}

// OptimizeVideoAssetNative abre um MP4, decodifica via H.264, redimensiona
// e codifica forçando keyint=1 antes de muxar o resultado de volta para MP4.
func OptimizeVideoAssetNative(inputPath, outputPath string, targetW, targetH int) error {
	inFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("falha ao abrir arquivo de entrada: %w", err)
	}
	defer inFile.Close()

	demuxer := demux.NewMP4Demuxer(inFile)
	tracks, err := demuxer.Probe()
	if err != nil {
		return fmt.Errorf("falha ao ler metadados do arquivo: %w", err)
	}

	var videoTrack *core.Track
	var audioTrack *core.Track
	videoTrackIndex := -1

	for i := range tracks {
		if tracks[i].Type == core.TrackTypeVideo {
			videoTrack = &tracks[i]
			videoTrackIndex = i
		} else if tracks[i].Type == core.TrackTypeAudio {
			audioTrack = &tracks[i]
		}
	}

	if videoTrack == nil {
		return fmt.Errorf("nenhuma trilha de vídeo encontrada no arquivo")
	}

	fps := 30
	if len(videoTrack.Samples) > 0 && videoTrack.Samples[0].Duration > 0 {
		fps = int(float64(videoTrack.Timescale) / float64(videoTrack.Samples[0].Duration))
	}
	if fps <= 0 {
		fps = 30
	}

	decoder := &core.SimH264Decoder{}
	defer decoder.Close()

	encoder := &core.SimH264Encoder{
		KeyintMax: 1, // Força todos-keyframes (keyint=1)
	}

	tmpVideoFile, err := os.CreateTemp("", "optimize_video_*.tmp")
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo temporário de vídeo: %w", err)
	}
	defer func() {
		tmpVideoFile.Close()
		os.Remove(tmpVideoFile.Name())
	}()

	var videoSamples []core.Sample
	var videoPTS int64 = 0
	videoTimescale := uint32(90000)
	if videoTrack.Timescale > 0 {
		videoTimescale = videoTrack.Timescale
	}
	videoFrameDuration := int64(videoTimescale) / int64(fps)
	if videoFrameDuration <= 0 {
		videoFrameDuration = 3000
	}

	var sps []byte
	var pps []byte

	writeVideoPacket := func(pkt *core.Packet) error {
		if pkt == nil {
			return nil
		}
		offset, err := tmpVideoFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		_, err = tmpVideoFile.Write(pkt.Data)
		if err != nil {
			return err
		}
		videoSamples = append(videoSamples, core.Sample{
			ID:         len(videoSamples) + 1,
			IsKeyframe: pkt.IsKeyframe,
			Offset:     offset,
			Size:       int64(len(pkt.Data)),
			Time:       videoPTS,
			Duration:   videoFrameDuration,
		})

		if len(sps) == 0 || len(pps) == 0 {
			nals := core.ParseAnnexBNalUnits(pkt.Data)
			for _, nal := range nals {
				if len(nal) > 0 {
					nalType := nal[0] & 0x1F
					if nalType == 7 {
						sps = append([]byte{}, nal...)
					} else if nalType == 8 {
						pps = append([]byte{}, nal...)
					}
				}
			}
		}
		videoPTS += videoFrameDuration
		return nil
	}

	scaler := &core.ScaleFilter{
		TargetW:  targetW,
		TargetH:  targetH,
		Bilinear: true,
	}

	for {
		pkt, err := demuxer.ReadPacket()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("falha ao ler pacote: %w", err)
		}

		if pkt.StreamIndex == videoTrackIndex {
			frame, err := decoder.Decode(pkt)
			core.GlobalPut(pkt.Data)
			if err != nil {
				return fmt.Errorf("falha ao decodificar frame: %w", err)
			}
			if frame == nil {
				continue
			}

			var rgbaData []byte
			if frame.Format == core.PixelFormatYUV420P {
				rgbaData = coreImage.ConvertYUV420pToRGBA(frame.Width, frame.Height, frame.Data)
				core.GlobalPut(frame.Data)
			} else if frame.Format == core.PixelFormatRGBA {
				rgbaData = frame.Data
			} else {
				return fmt.Errorf("formato de vídeo não suportado: %s", frame.Format)
			}

			rgbaFrame := &core.VideoFrame{
				Width:  frame.Width,
				Height: frame.Height,
				Format: core.PixelFormatRGBA,
				Data:   rgbaData,
			}

			scaledFrame, err := scaler.Process(rgbaFrame)
			core.GlobalPut(rgbaFrame.Data)
			if err != nil {
				return fmt.Errorf("falha ao redimensionar frame: %w", err)
			}

			outPkt, err := encoder.Encode(scaledFrame)
			if err != nil {
				return fmt.Errorf("falha ao codificar frame: %w", err)
			}
			if outPkt != nil {
				if err := writeVideoPacket(outPkt); err != nil {
					return err
				}
			}
		} else {
			core.GlobalPut(pkt.Data)
		}
	}

	for {
		outPkt, err := encoder.Encode(nil)
		if err != nil {
			return fmt.Errorf("falha ao limpar buffer do encoder de vídeo: %w", err)
		}
		if outPkt == nil {
			break
		}
		if err := writeVideoPacket(outPkt); err != nil {
			return err
		}
	}

	encoder.Close()

	if len(videoSamples) == 0 {
		return fmt.Errorf("nenhum frame de vídeo processado com sucesso")
	}

	if len(sps) == 0 {
		sps = []byte{0x67, 0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x50, 0x05, 0xbb, 0x01, 0x10, 0x00, 0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x03, 0x20, 0xf1, 0x83, 0x19, 0x60}
	}
	if len(pps) == 0 {
		pps = []byte{0x68, 0xeb, 0xec, 0xb2, 0x2c}
	}

	optVideoTrack := core.Track{
		ID:          1,
		Type:        core.TrackTypeVideo,
		Timescale:   videoTimescale,
		Duration:    uint64(videoPTS),
		Width:       uint32(targetW),
		Height:      uint32(targetH),
		CodecTag:    "avc1",
		Samples:     videoSamples,
		Hdlr:        mux.DefaultVideoHdlr(),
		MediaHeader: mux.DefaultVideoMediaHeader(),
		Stsd:        mux.MakeH264Stsd(targetW, targetH, sps, pps),
	}

	var outTracks []core.Track
	outTracks = append(outTracks, optVideoTrack)

	if audioTrack != nil {
		optAudioTrack := *audioTrack
		optAudioTrack.ID = 2
		outTracks = append(outTracks, optAudioTrack)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo de saída: %w", err)
	}
	defer outFile.Close()

	muxer := mux.NewMP4Muxer(outFile)
	if err := muxer.WriteHeader(outTracks); err != nil {
		return fmt.Errorf("falha ao escrever cabeçalho no container final: %w", err)
	}

	sources := make(map[int]io.ReadSeeker)
	sources[0] = tmpVideoFile

	if audioTrack != nil {
		audioFile, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("falha ao abrir arquivo para cópia da trilha de áudio: %w", err)
		}
		defer audioFile.Close()
		sources[1] = audioFile
	}

	interleaved := mux.BuildInterleavedOrder(outTracks)
	bufSize := 65536
	copyBuf := core.GlobalGet(bufSize)
	defer core.GlobalPut(copyBuf)

	for _, is := range interleaved {
		src := sources[is.TrackIndex]
		origSample := outTracks[is.TrackIndex].Samples[is.SampleIndex]

		_, err = src.Seek(origSample.Offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("falha ao buscar offset %d: %w", origSample.Offset, err)
		}

		remaining := origSample.Size
		for remaining > 0 {
			toRead := int64(bufSize)
			if remaining < toRead {
				toRead = remaining
			}

			_, err = io.ReadFull(src, copyBuf[:toRead])
			if err != nil {
				return fmt.Errorf("falha ao ler bloco: %w", err)
			}

			pkt := &core.Packet{Data: copyBuf[:toRead]}
			if err := muxer.WritePacket(pkt); err != nil {
				return fmt.Errorf("falha ao escrever pacote: %w", err)
			}
			remaining -= toRead
		}
	}

	return muxer.WriteTrailer()
}
