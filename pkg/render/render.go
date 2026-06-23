package render

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"crom-video-gen/internal/execs"
	"crom-video-gen/pkg/types"

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

// RenderScene gera um arquivo MP4 intermediário para uma única cena usando headless Chrome e FFmpeg pipe
func RenderScene(ctx context.Context, logger *slog.Logger, chromeCtx context.Context, localAddr string, projectRoot string, cena *types.Cena, global *types.GlobalConfig, audioPath string, duration float64, outputPath string) error {
	ffmpegPath := execs.ResolveFFmpegPath()
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
			// Caso não seja absoluto começando com projectRoot, mas seja absoluto, tenta deixar relativo se possível
			// Se já for relativo (ex: assets/...), garante que inicie com / para ser resolvido na raiz do servidor
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

	// Injeta o JSON usando a função window.setupTemplate (com decodificação UTF-8 robusta para Base64)
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
			window.setupFinished = false;
			const images = Array.from(document.querySelectorAll('img'));
			const videos = Array.from(document.querySelectorAll('video'));
			
			let pending = images.length + videos.length;
			const checkReady = () => {
				if (pending <= 0) {
					// Espera dois frames de animação para garantir o primeiro paint e layout
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
					// Força recarregamento caso esteja no cache ou pendente
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

	// Configura a pipeline do FFmpeg para receber frames via stdin (image2pipe)
	args := []string{
		"-y",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-r", fmt.Sprintf("%d", global.FPS),
		"-i", "-", // Entrada via stdin
	}
	if audioPath != "" {
		args = append(args, "-i", audioPath)
	}

	// Mapeamento de vídeo do stdin
	args = append(args, "-map", "0:v")

	if audioPath != "" {
		// Mapeamento de áudio do arquivo de narração
		args = append(args,
			"-map", "1:a",
			"-c:a", "aac",
			"-b:a", "192k",
			"-ar", fmt.Sprintf("%d", global.Audio.SampleRate),
			"-ac", fmt.Sprintf("%d", global.Audio.Canais),
		)
	}

	// Configuração de codificação de vídeo final
	args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-crf", "18")
	
	if w != 1920 || h != 1080 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d", w, h))
	}
	args = append(args,
		"-pix_fmt", "yuv420p",
		"-t", fmt.Sprintf("%.3f", duration),
		outputPath,
	)

	logger.Debug("Executando FFmpeg para cena", "comando", ffmpegPath+" "+strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("falha ao obter stdin do FFmpeg: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar FFmpeg: %w (stderr: %s)", err, stderr.String())
	}

	// Channel para processamento concorrente do stdin do FFmpeg
	// Limitamos o buffer a 8 frames para evitar uso excessivo de RAM (8 * ~500KB = ~4MB por cena)
	frameChan := make(chan []byte, 8)
	errChan := make(chan error, 1)

	go func() {
		defer stdin.Close()
		for img := range frameChan {
			if _, err := stdin.Write(img); err != nil {
				errChan <- err
				return
			}
		}
		errChan <- nil
	}()

	// Loop frame-a-frame de captura otimizado
	totalFrames := int(duration * float64(global.FPS))
	if totalFrames < 1 {
		totalFrames = 1
	}

	var renderErr error
	for f := 0; f < totalFrames; f++ {
		// Verifica antes de prosseguir se a goroutine de escrita encontrou algum erro
		select {
		case writeErr := <-errChan:
			if writeErr != nil {
				renderErr = fmt.Errorf("falha precoce na escrita de frames no pipe do FFmpeg: %w (stderr: %s)", writeErr, stderr.String())
				break
			}
		default:
		}
		if renderErr != nil {
			break
		}

		timeSeconds := float64(f) / float64(global.FPS)

		// Script JavaScript para buscar o frame e sinalizar quando terminar o seek
		var seekScript string
		if hasVideos {
			seekScript = fmt.Sprintf(`
				new Promise((resolve) => {
					const videos = Array.from(document.querySelectorAll('video'));
					let pending = 0;
					let resolved = false;
					
					const checkResolve = () => {
						if (pending === 0 && !resolved) {
							resolved = true;
							resolve();
						}
					};

					videos.forEach(v => {
						pending++;
						const onSeeked = () => {
							pending--;
							checkResolve();
						};
						v.addEventListener('seeked', onSeeked, { once: true });
						// Timeout backup caso não dispare
						setTimeout(() => {
							if (v.seeking === false) {
								v.removeEventListener('seeked', onSeeked);
								pending--;
								checkResolve();
							}
						}, 250);
					});

					window.seekTo(%f, %f);
					checkResolve(); // Para o caso de não haver vídeos
				});
			`, timeSeconds, duration)
		} else {
			seekScript = fmt.Sprintf(`window.seekTo(%f, %f);`, timeSeconds, duration)
		}

		// Executa o script de seek usando Evaluate. Para Promise, aguarda finalizar.
		err = chromedp.Run(sceneCtx, chromedp.Evaluate(seekScript, nil))
		if err != nil {
			renderErr = fmt.Errorf("falha ao disparar seekTo no frame %d: %w", f, err)
			break
		}

		// Captura o frame como screenshot JPEG com qualidade 85 (ótimo compromisso qualidade/velocidade)
		var imageBuf []byte
		err = chromedp.Run(sceneCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var captureErr error
			imageBuf, captureErr = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(85).
				Do(ctx)
			return captureErr
		}))
		if err != nil {
			renderErr = fmt.Errorf("falha ao capturar screenshot no frame %d: %w", f, err)
			break
		}

		// Envia os bytes para serem escritos no stdin concorrentemente
		select {
		case frameChan <- imageBuf:
		case writeErr := <-errChan:
			if writeErr != nil {
				renderErr = fmt.Errorf("falha ao enviar frame %d para gravação no FFmpeg: %w (stderr: %s)", f, writeErr, stderr.String())
				break
			}
		}
		if renderErr != nil {
			break
		}
	}

	// Fecha o canal de frames para sinalizar término da gravação
	close(frameChan)

	// Aguarda o término da goroutine escritora se não houve erros prévios
	if renderErr == nil {
		if writeErr := <-errChan; writeErr != nil {
			renderErr = fmt.Errorf("falha ao finalizar escrita de frames no pipe do FFmpeg: %w (stderr: %s)", writeErr, stderr.String())
		}
	}

	if renderErr != nil {
		return renderErr
	}

	// Aguarda o término do processo FFmpeg
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("FFmpeg falhou ao concluir renderização da cena %d: %w (stderr: %s)", cena.ID, err, stderr.String())
	}

	return nil
}

// ConcatScenes junta múltiplos vídeos usando o concat demuxer e aplica a trilha sonora final
func ConcatScenes(ctx context.Context, logger *slog.Logger, sceneFiles []string, soundtrackPath string, globalTrackVolume float64, audioConf *types.AudioConfig, tempDir string, outputPath string) error {
	ffmpegPath := execs.ResolveFFmpegPath()

	// 1. Cria o arquivo txt com a lista das cenas para o demuxer concat
	listFilePath := filepath.Join(tempDir, "concat_list.txt")
	var fileListContent strings.Builder
	for _, f := range sceneFiles {
		// O concat demuxer exige caminhos absolutos ou relativos simples e aspas simples escapadas
		absPath, err := filepath.Abs(f)
		if err != nil {
			return fmt.Errorf("falha ao resolver caminho absoluto de '%s': %w", f, err)
		}
		fileListContent.WriteString(fmt.Sprintf("file '%s'\n", strings.ReplaceAll(absPath, "'", "'\\''")))
	}

	if err := os.WriteFile(listFilePath, []byte(fileListContent.String()), 0644); err != nil {
		return fmt.Errorf("falha ao criar arquivo de concatenação: %w", err)
	}

	// Primeiro passo: Concatena as cenas num vídeo provisório sem a trilha sonora
	tempConcatOutput := filepath.Join(tempDir, "concatenated_no_music.mp4")
	concatArgs := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listFilePath,
		"-c", "copy",
		tempConcatOutput,
	}

	logger.Debug("Concatenando cenas", "comando", ffmpegPath+" "+strings.Join(concatArgs, " "))
	cmdConcat := exec.CommandContext(ctx, ffmpegPath, concatArgs...)
	var stderrConcat bytes.Buffer
	cmdConcat.Stderr = &stderrConcat
	if err := cmdConcat.Run(); err != nil {
		return fmt.Errorf("falha ao concatenar cenas: %w (stderr: %s)", err, stderrConcat.String())
	}

	// Segundo passo: Muxa a trilha sonora em loop com os áudios de narração do vídeo concatenado
	var muxArgs []string
	if soundtrackPath != "" {
		audioFilter := fmt.Sprintf("[1:a]volume=%.3f[music]; [0:a][music]amix=inputs=2:duration=first:dropout_transition=2", globalTrackVolume)
		if audioConf.NormalizarVolume {
			audioFilter += ",loudnorm[audio_final]"
		} else {
			audioFilter += "[audio_final]"
		}

		muxArgs = []string{
			"-y",
			"-i", tempConcatOutput,
			"-stream_loop", "-1", // loop infinito na música de fundo
			"-i", soundtrackPath,
			"-filter_complex", audioFilter,
			"-map", "0:v",
			"-map", "[audio_final]",
			"-c:v", "copy", // copia o fluxo de vídeo sem re-encodificar (super rápido)
			"-c:a", audioConf.Codec,
			"-ar", fmt.Sprintf("%d", audioConf.SampleRate),
			"-ac", fmt.Sprintf("%d", audioConf.Canais),
			"-b:a", audioConf.Bitrate,
			outputPath,
		}
	} else {
		muxArgs = []string{
			"-y",
			"-i", tempConcatOutput,
			"-map", "0:v",
			"-map", "0:a",
			"-c:v", "copy",
		}
		if audioConf.NormalizarVolume {
			muxArgs = append(muxArgs, "-af", "loudnorm")
			muxArgs = append(muxArgs,
				"-c:a", audioConf.Codec,
				"-ar", fmt.Sprintf("%d", audioConf.SampleRate),
				"-ac", fmt.Sprintf("%d", audioConf.Canais),
				"-b:a", audioConf.Bitrate,
			)
		} else {
			muxArgs = append(muxArgs, "-c:a", "copy")
		}
		muxArgs = append(muxArgs, outputPath)
	}

	logger.Debug("Adicionando trilha sonora", "comando", ffmpegPath+" "+strings.Join(muxArgs, " "))
	cmdMux := exec.CommandContext(ctx, ffmpegPath, muxArgs...)
	var stderrMux bytes.Buffer
	cmdMux.Stderr = &stderrMux
	if err := cmdMux.Run(); err != nil {
		return fmt.Errorf("falha ao mixar trilha sonora: %w (stderr: %s)", err, stderrMux.String())
	}

	return nil
}
