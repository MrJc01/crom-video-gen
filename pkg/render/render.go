package render

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"crom-video-gen/pkg/types"

	"cromedia/core"
	"cromedia/core/demux"
	"cromedia/core/filters/audio"
	"cromedia/core/mux"
	"cromedia/core/pipeline"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)


// EscapeDrawtext sanitiza o texto para uso no filtro drawtext do FFmpeg
func EscapeDrawtext(text string) string {
	t := strings.ReplaceAll(text, "\\", "\\\\")
	t = strings.ReplaceAll(t, "'", "'\\''")
	t = strings.ReplaceAll(t, ":", "\\:")
	return t
}

// WrapText quebra o texto em linhas de no máximo maxLen caracteres
func WrapText(text string, maxLen int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	var currentLine []string
	currentLen := 0

	for _, word := range words {
		// Conta caracteres especiais
		if currentLen+len(word)+1 > maxLen {
			lines = append(lines, strings.Join(currentLine, " "))
			currentLine = []string{word}
			currentLen = len(word)
		} else {
			currentLine = append(currentLine, word)
			currentLen += len(word) + 1
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, " "))
	}
	return strings.Join(lines, "\n")
}

// RenderScene gera um arquivo MP4 intermediário para uma única cena usando headless Chrome e CroMedia Pipeline
func RenderScene(ctx context.Context, logger *slog.Logger, chromeCtx context.Context, localAddr string, projectRoot string, cena *types.Cena, global *types.GlobalConfig, audioPath string, duration float64, outputPath string) error {
	w, h, err := global.GetResolutionWidthAndHeight()
	if err != nil {
		return fmt.Errorf("resolução inválida: %w", err)
	}

	// Ajusta o caminho dos ativos na struct da cena para rotas relativas do servidor web
	browserCena := *cena
	browserCena.Ativos = make(map[string]types.Ativo)
	for key, ativo := range cena.Ativos {
		relPath := ativo.Caminho
		if strings.HasPrefix(ativo.Caminho, projectRoot) {
			relPath = strings.TrimPrefix(ativo.Caminho, projectRoot)
			if !strings.HasPrefix(relPath, "/") {
				relPath = "/" + relPath
			}
		} else {
			if !strings.HasPrefix(relPath, "/") {
				relPath = "/" + relPath
			}
		}
		browserCena.Ativos[key] = types.Ativo{
			Tipo:    ativo.Tipo,
			Caminho: relPath,
		}
	}

	// Verifica se a cena contém ativos de vídeo para otimização do seek
	hasVideos := false
	for _, ativo := range cena.Ativos {
		if ativo.Tipo == "video" {
			hasVideos = true
			break
		}
	}

	// Converte a struct da cena para JSON e codifica em Base64
	cenaBytes, err := json.Marshal(browserCena)
	if err != nil {
		return fmt.Errorf("falha ao codificar cena para JSON: %w", err)
	}
	cenaBase64 := base64.StdEncoding.EncodeToString(cenaBytes)

	// Inicializa o contexto do browser filho (nova aba no Chrome existente)
	sceneCtx, sceneCancel := chromedp.NewContext(chromeCtx)
	defer sceneCancel()

	// Navega para o template correspondente
	templateURL := fmt.Sprintf("http://%s/templates/%s/index.html", localAddr, cena.Template.ID)
	logger.Debug("Navegando para o template da cena", "url", templateURL)

	err = chromedp.Run(sceneCtx,
		chromedp.Navigate(templateURL),
		chromedp.EmulateViewport(1920, 1080),
	)
	if err != nil {
		return fmt.Errorf("falha ao navegar para o template %s: %w", cena.Template.ID, err)
	}

	// Injeta o JSON usando a função window.setupTemplate
	setupScript := fmt.Sprintf(`
		(function() {
			const b64 = '%s';
			const bin = atob(b64);
			const bytes = new Uint8Array(bin.length);
			for (let i = 0; i < bin.length; i++) {
				bytes[i] = bin.charCodeAt(i);
			}
			const jsonStr = new TextDecoder().decode(bytes);
			window.setupTemplate(jsonStr);
		})()
	`, cenaBase64)
	err = chromedp.Run(sceneCtx, chromedp.Evaluate(setupScript, nil))
	if err != nil {
		return fmt.Errorf("falha ao configurar o template via JS: %w", err)
	}

	// Aguarda o carregamento completo de todos os ativos (imagens e vídeos) no DOM
	waitAssetsScript := `
		(function() {
			if (!window._currentTimePatched) {
				window._currentTimePatched = true;
				try {
					const descriptor = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'currentTime');
					if (descriptor && descriptor.set) {
						const originalSet = descriptor.set;
						const originalGet = descriptor.get;
						Object.defineProperty(HTMLMediaElement.prototype, 'currentTime', {
							configurable: true,
							enumerable: true,
							get: function() {
								return originalGet.call(this);
							},
							set: function(val) {
								let target = val;
								if (this.loop && this.duration && this.duration > 0) {
									target = val % this.duration;
								}
								originalSet.call(this, target);
							}
						});
					}
				} catch (e) {
					console.error("Falha ao interceptar currentTime:", e);
				}

				try {
					HTMLMediaElement.prototype.play = function() {
						return Promise.resolve();
					};
				} catch (e) {
					console.error("Falha ao desativar play():", e);
				}
			}

			window.setupFinished = false;
			const images = Array.from(document.querySelectorAll('img'));
			const videos = Array.from(document.querySelectorAll('video'));
			
			videos.forEach(v => {
				try {
					v.pause();
					v.autoplay = false;
				} catch (e) {}
			});

			let pending = images.length + videos.length;
			const checkReady = () => {
				if (pending <= 0) {
					requestAnimationFrame(() => {
						requestAnimationFrame(() => {
							window.setupFinished = true;
						});
					});
				}
			};
			
			images.forEach(img => {
				if (img.complete && img.naturalWidth > 0) {
					pending--;
				} else {
					img.addEventListener('load', () => {
						pending--;
						checkReady();
					}, { once: true });
					img.addEventListener('error', () => {
						pending--;
						checkReady();
					}, { once: true });
					if (img.src) {
						const src = img.src;
						img.src = '';
						img.src = src;
					}
				}
			});
			
			videos.forEach(v => {
				if (v.readyState >= 2) {
					pending--;
				} else {
					v.addEventListener('loadeddata', () => {
						pending--;
						checkReady();
					}, { once: true });
					v.addEventListener('error', () => {
						pending--;
						checkReady();
					}, { once: true });
					v.load();
				}
			});
			
			checkReady();
		})();
	`
	err = chromedp.Run(sceneCtx, chromedp.Evaluate(waitAssetsScript, nil))
	if err != nil {
		return fmt.Errorf("falha ao injetar script de espera de ativos: %w", err)
	}

	// Polling para esperar até 2 segundos pelo carregamento dos ativos
	var setupFinished bool
	for i := 0; i < 200; i++ { // 200 * 10ms = 2s
		err = chromedp.Run(sceneCtx, chromedp.Evaluate("window.setupFinished", &setupFinished))
		if err != nil {
			return fmt.Errorf("falha ao checar status de carregamento: %w", err)
		}
		if setupFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	logger.Debug("Status de inicialização dos ativos da cena", "cena_id", cena.ID, "carregado", setupFinished)



	// Prepara a trilha de narração se fornecida
	var audioTrack *core.Track
	var audioFile *os.File
	if audioPath != "" {
		audioFile, err = os.Open(audioPath)
		if err != nil {
			return fmt.Errorf("falha ao abrir arquivo de narração: %w", err)
		}
		defer audioFile.Close()

		format, err := demux.SniffFormat(audioFile)
		if err != nil {
			return fmt.Errorf("falha ao identificar formato da narração: %w", err)
		}

		audioDemuxer, err := demux.NewDemuxerFromFormat(format, audioFile)
		if err != nil {
			return fmt.Errorf("falha ao criar demuxer para narração: %w", err)
		}
		defer audioDemuxer.Close()

		tracks, err := audioDemuxer.Probe()
		if err != nil {
			return fmt.Errorf("falha ao ler metadados da narração: %w", err)
		}

		if len(tracks) > 0 {
			audioTrack = &tracks[0]
		}
	}

	// Instancia o codificador H.264
	videoEncoder := core.NewSimH264Encoder(int(global.FPS), 0)
	defer videoEncoder.Close()

	frameChan := make(chan interface{}, 8)
	errChan := make(chan error, 1)

	// Inicializa o Pipeline CroMedia de gravação de cena
	go func() {
		errChan <- pipeline.RenderScenePipeline(
			outputPath,
			w, h,
			int(global.FPS),
			videoEncoder,
			frameChan,
			audioTrack,
			audioFile,
		)
	}()

	totalFrames := int(duration * float64(global.FPS))
	if totalFrames < 1 {
		totalFrames = 1
	}

	// Loop frame-a-frame de captura
	sceneTimeout := time.Duration(totalFrames)*2*time.Second + 30*time.Second
	runCtx, runCancel := context.WithTimeout(sceneCtx, sceneTimeout)
	defer runCancel()

	err = chromedp.Run(runCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		for f := 0; f < totalFrames; f++ {
			timeSeconds := float64(f) / float64(global.FPS)

			var seekScript string
			if hasVideos {
				seekScript = fmt.Sprintf(`
					new Promise((resolve) => {
						const videos = Array.from(document.querySelectorAll('video'));
						const activeVideos = videos.filter(v => {
							let targetTime = %f;
							if (v.loop && v.duration && v.duration > 0) {
								targetTime = targetTime %% v.duration;
							} else {
								targetTime = Math.min(targetTime, v.duration || Infinity);
							}
							return Math.abs(v.currentTime - targetTime) > 0.001;
						});

						if (activeVideos.length === 0) {
							window.seekTo(%f, %f);
							resolve();
							return;
						}

						let resolved = false;
						const done = () => {
							if (!resolved) {
								resolved = true;
								resolve();
							}
						};

						const timer = setTimeout(done, 100);

						let pending = 0;
						activeVideos.forEach(v => {
							pending++;
							const onSeeked = () => {
								pending--;
								if (pending === 0) {
									clearTimeout(timer);
									done();
								}
							};
							v.addEventListener('seeked', onSeeked, { once: true });
						});

						window.seekTo(%f, %f);
					});
				`, timeSeconds, timeSeconds, duration, timeSeconds, duration)
			} else {
				seekScript = fmt.Sprintf(`window.seekTo(%f, %f);`, timeSeconds, duration)
			}

			var evalErr error
			evalErr = chromedp.Evaluate(seekScript, nil).Do(ctx)
			if evalErr != nil {
				return fmt.Errorf("falha ao realizar seek no frame %d: %w", f, evalErr)
			}

			var imageBuf []byte
			imageBuf, evalErr = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(75).
				Do(ctx)
			if evalErr != nil {
				return fmt.Errorf("falha ao capturar screenshot no frame %d: %w", f, evalErr)
			}

			select {
			case frameChan <- imageBuf:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}))

	close(frameChan)

	renderErr := <-errChan

	if err != nil {
		return err
	}

	if renderErr != nil {
		return fmt.Errorf("falha ao concluir processamento de frames: %w", renderErr)
	}

	return nil
}

// ConcatScenes junta múltiplos vídeos usando o concat demuxer e aplica a trilha sonora final nativamente
func ConcatScenes(ctx context.Context, logger *slog.Logger, sceneFiles []string, soundtrackPath string, globalTrackVolume float64, audioConf *types.AudioConfig, tempDir string, outputPath string) error {
	// Primeiro passo: Concatena as cenas num vídeo provisório sem a trilha sonora usando stream copy nativo do CroMedia
	tempConcatOutput := filepath.Join(tempDir, "concatenated_no_music.mp4")
	logger.Debug("Concatenando cenas de forma nativa com CroMedia", "arquivos", sceneFiles)
	
	if err := mux.ConcatMP4Files(tempConcatOutput, sceneFiles); err != nil {
		return fmt.Errorf("falha ao concatenar cenas de forma nativa: %w", err)
	}

	// Segundo passo: Muxa/mixa a trilha de música com os áudios de narração do vídeo concatenado
	logger.Debug("Mixando trilha sonora e narração nativamente com CroMedia", "trilha", soundtrackPath, "volume", globalTrackVolume)
	if err := mixAudioTracksNative(tempConcatOutput, soundtrackPath, outputPath, globalTrackVolume, audioConf); err != nil {
		return fmt.Errorf("falha ao mixar trilha sonora: %w", err)
	}

	return nil
}

// mixAudioTracksNative decodifica, mixa e codifica trilhas de áudio nativamente
func mixAudioTracksNative(videoMP4, soundtrackPath, outputMP4 string, globalTrackVolume float64, audioConf *types.AudioConfig) error {
	videoFile, err := os.Open(videoMP4)
	if err != nil {
		return fmt.Errorf("falha ao abrir arquivo de vídeo: %w", err)
	}
	defer videoFile.Close()

	demuxer := demux.NewMP4Demuxer(videoFile)
	tracks, err := demuxer.Probe()
	if err != nil {
		return fmt.Errorf("falha ao ler metadados do vídeo: %w", err)
	}

	var videoTrack *core.Track
	var voiceTrack *core.Track
	voiceTrackIndex := -1

	for i := range tracks {
		if tracks[i].Type == core.TrackTypeVideo {
			videoTrack = &tracks[i]
		} else if tracks[i].Type == core.TrackTypeAudio {
			voiceTrack = &tracks[i]
			voiceTrackIndex = i
		}
	}

	if videoTrack == nil {
		return errors.New("nenhuma trilha de vídeo encontrada no MP4")
	}

	if voiceTrack == nil && soundtrackPath == "" {
		src, err := os.Open(videoMP4)
		if err != nil {
			return fmt.Errorf("falha ao abrir vídeo sem áudio: %w", err)
		}
		defer src.Close()
		dst, err := os.Create(outputMP4)
		if err != nil {
			return fmt.Errorf("falha ao criar contêiner de saída sem áudio: %w", err)
		}
		defer dst.Close()
		if _, err = io.Copy(dst, src); err != nil {
			return fmt.Errorf("falha ao copiar vídeo sem áudio para o destino: %w", err)
		}
		return nil
	}

	var voiceFrame *core.AudioFrame
	var voiceFrames []*core.AudioFrame

	if voiceTrack != nil {
		aacDec := &core.SimAACDecoder{}
		if len(voiceTrack.CodecPrivate) > 0 {
			_ = aacDec.Init(voiceTrack.CodecPrivate)
		}
		defer aacDec.Close()

		for {
			pkt, err := demuxer.ReadPacket()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("falha ao ler pacote de áudio: %w", err)
			}

			if pkt.StreamIndex == voiceTrackIndex {
				f, err := aacDec.Decode(pkt)
				core.GlobalPut(pkt.Data)
				if err != nil {
					return fmt.Errorf("falha ao decodificar pacote AAC: %w", err)
				}
				if f != nil {
					voiceFrames = append(voiceFrames, f)
				}
			} else {
				core.GlobalPut(pkt.Data)
			}
		}

		if len(voiceFrames) > 0 {
			totalSamples := 0
			for _, f := range voiceFrames {
				totalSamples += len(f.Data)
			}
			voiceData := make([]float32, totalSamples)
			offset := 0
			for _, f := range voiceFrames {
				copy(voiceData[offset:], f.Data)
				offset += len(f.Data)
			}
			voiceFrame = &core.AudioFrame{
				Channels:   voiceFrames[0].Channels,
				SampleRate: voiceFrames[0].SampleRate,
				Data:       voiceData,
			}
		}
	}

	var musicFrame *core.AudioFrame
	if soundtrackPath != "" {
		stFile, err := os.Open(soundtrackPath)
		if err != nil {
			return fmt.Errorf("falha ao abrir trilha sonora: %w", err)
		}
		defer stFile.Close()

		format, err := demux.SniffFormat(stFile)
		if err != nil {
			return fmt.Errorf("falha ao farejar formato da trilha: %w", err)
		}

		if format == "mp3" {
			musicFrame, err = core.DecodeMP3File(soundtrackPath)
			if err != nil {
				return fmt.Errorf("falha ao decodificar MP3 de trilha sonora: %w", err)
			}
		} else if format == "wav" {
			wavDemux := demux.NewWAVDemuxer(stFile)
			wavTracks, err := wavDemux.Probe()
			if err != nil {
				return fmt.Errorf("falha ao ler metadados de WAV: %w", err)
			}
			if len(wavTracks) == 0 {
				return fmt.Errorf("nenhuma trilha de áudio encontrada no WAV")
			}

			codec := &core.PCMAudioCodec{
				Channels:   2, // Padrão Estéreo para RIFF/WAV no nosso sistema
				SampleRate: int(wavTracks[0].Timescale),
				BitDepth:   16,
			}

			var wavData []float32
			for {
				pkt, err := wavDemux.ReadPacket()
				if err != nil {
					if err == io.EOF {
						break
					}
					return fmt.Errorf("falha ao ler pacote WAV: %w", err)
				}
				f, err := codec.Decode(pkt)
				core.GlobalPut(pkt.Data)
				if err != nil {
					return fmt.Errorf("falha ao decodificar PCM WAV: %w", err)
				}
				if f != nil {
					wavData = append(wavData, f.Data...)
				}
			}
			musicFrame = &core.AudioFrame{
				Channels:   codec.Channels,
				SampleRate: codec.SampleRate,
				Data:       wavData,
			}
		} else {
			return fmt.Errorf("formato de trilha sonora não suportado: %s", format)
		}
	}

	// Mixa as trilhas
	mixedFrame := audio.MixAudioFrames(voiceFrame, musicFrame, 1.0, float32(globalTrackVolume), true)
	if mixedFrame == nil {
		return errors.New("a mixagem final do áudio resultou em frame vazio")
	}

	// Normaliza áudio se configurado
	if audioConf.NormalizarVolume {
		normalizer := core.NewPredictiveGainNormalizer(-1.0, 0.05)
		mixedFrame, err = normalizer.Process(mixedFrame)
		if err != nil {
			return fmt.Errorf("falha na normalização final de loudness: %w", err)
		}
	}

	targetChannels := mixedFrame.Channels
	targetSampleRate := mixedFrame.SampleRate

	// Codifica para AAC
	aacEnc := &core.SimAACEncoder{}
	defer aacEnc.Close()

	tmpAudioFile, err := os.CreateTemp("", "mix_audio_*.tmp")
	if err != nil {
		return fmt.Errorf("falha ao criar cache temporário de áudio: %w", err)
	}
	defer func() {
		tmpAudioFile.Close()
		os.Remove(tmpAudioFile.Name())
	}()

	var audioSamples []core.Sample
	var audioPTS int64 = 0
	audioFrameDuration := int64(1024)

	writeAudioPacket := func(pkt *core.Packet) error {
		if pkt == nil {
			return nil
		}
		offset, err := tmpAudioFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		_, err = tmpAudioFile.Write(pkt.Data)
		if err != nil {
			return err
		}
		audioSamples = append(audioSamples, core.Sample{
			ID:         len(audioSamples) + 1,
			IsKeyframe: true,
			Offset:     offset,
			Size:       int64(len(pkt.Data)),
			Time:       audioPTS,
			Duration:   audioFrameDuration,
		})
		audioPTS += audioFrameDuration
		return nil
	}

	chunkSize := 1024 * targetChannels
	for i := 0; i < len(mixedFrame.Data); i += chunkSize {
		end := i + chunkSize
		if end > len(mixedFrame.Data) {
			end = len(mixedFrame.Data)
		}
		chunkData := mixedFrame.Data[i:end]

		chunkFrame := &core.AudioFrame{
			Channels:   targetChannels,
			SampleRate: targetSampleRate,
			Data:       chunkData,
		}

		pkt, err := aacEnc.Encode(chunkFrame)
		if err != nil {
			return fmt.Errorf("falha ao codificar fragmento de áudio: %w", err)
		}
		if pkt != nil {
			if err := writeAudioPacket(pkt); err != nil {
				return err
			}
		}
	}

	for {
		pkt, err := aacEnc.Encode(nil)
		if err != nil {
			return fmt.Errorf("falha ao limpar buffer do encoder de áudio: %w", err)
		}
		if pkt == nil {
			break
		}
		if err := writeAudioPacket(pkt); err != nil {
			return err
		}
	}

	if len(audioSamples) == 0 {
		return errors.New("nenhum frame de áudio foi codificado com sucesso")
	}

	makeAudioSpecificConfig := func(sampleRate int, channels int) []byte {
		srIndex := 4 // Default 44100
		switch sampleRate {
		case 96000: srIndex = 0
		case 88200: srIndex = 1
		case 64000: srIndex = 2
		case 48000: srIndex = 3
		case 44100: srIndex = 4
		case 32000: srIndex = 5
		case 24000: srIndex = 6
		case 22050: srIndex = 7
		case 16000: srIndex = 8
		case 12000: srIndex = 9
		case 11025: srIndex = 10
		case 8000:  srIndex = 11
		case 7350:  srIndex = 12
		}

		b0 := byte(2<<3) | byte(srIndex>>1)
		b1 := byte((srIndex&1)<<7) | byte((channels&15)<<3)
		return []byte{b0, b1}
	}

	optAudioTrack := core.Track{
		ID:          2,
		Type:        core.TrackTypeAudio,
		Timescale:   uint32(targetSampleRate),
		Duration:    uint64(audioPTS),
		CodecTag:    "mp4a",
		Samples:     audioSamples,
		Hdlr:        mux.DefaultAudioHdlr(),
		MediaHeader: mux.DefaultAudioMediaHeader(),
		Stsd:        mux.MakeAACStsd(targetSampleRate, targetChannels, makeAudioSpecificConfig(targetSampleRate, targetChannels)),
	}

	// Copia trilha de vídeo sem re-encoding
	optVideoTrack := *videoTrack
	optVideoTrack.ID = 1

	var outTracks []core.Track
	outTracks = append(outTracks, optVideoTrack)
	outTracks = append(outTracks, optAudioTrack)

	outFile, err := os.Create(outputMP4)
	if err != nil {
		return fmt.Errorf("falha ao criar contêiner MP4 final: %w", err)
	}
	defer outFile.Close()

	muxer := mux.NewMP4Muxer(outFile)
	if err := muxer.WriteHeader(outTracks); err != nil {
		return fmt.Errorf("falha ao escrever cabeçalhos no MP4 final: %w", err)
	}

	sources := make(map[int]io.ReadSeeker)

	videoSourceFile, err := os.Open(videoMP4)
	if err != nil {
		return fmt.Errorf("falha ao abrir vídeo temporário para remuxer: %w", err)
	}
	defer videoSourceFile.Close()
	sources[0] = videoSourceFile
	sources[1] = tmpAudioFile

	interleaved := mux.BuildInterleavedOrder(outTracks)
	bufSize := 65536
	copyBuf := core.GlobalGet(bufSize)
	defer core.GlobalPut(copyBuf)

	for _, is := range interleaved {
		src := sources[is.TrackIndex]
		origSample := outTracks[is.TrackIndex].Samples[is.SampleIndex]

		_, err = src.Seek(origSample.Offset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("falha ao ler offset %d: %w", origSample.Offset, err)
		}

		remaining := origSample.Size
		for remaining > 0 {
			toRead := int64(bufSize)
			if remaining < toRead {
				toRead = remaining
			}

			_, err = io.ReadFull(src, copyBuf[:toRead])
			if err != nil {
				return fmt.Errorf("falha ao copiar frame do buffer: %w", err)
			}

			pkt := &core.Packet{Data: copyBuf[:toRead]}
			if err := muxer.WritePacket(pkt); err != nil {
				return fmt.Errorf("falha ao muxar pacote final: %w", err)
			}
			remaining -= toRead
		}
	}

	return muxer.WriteTrailer()
}
