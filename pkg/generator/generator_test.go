package generator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateVideo_E2E(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "generator_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Cria uma mini configuração JSON válida de teste com múltiplos ativos
	jsonConfig := `{
		"projeto": {
			"titulo": "Documentação Automatizada - Teste Integrado",
			"configuracoes_globais": {
				"resolucao": "640x360",
				"fps": 30,
				"formato_saida": "mp4",
				"audio": {
					"sample_rate": 44100,
					"bitrate": "128k",
					"canais": 2,
					"codec": "aac",
					"normalizar_volume": true
				}
			},
			"trilha_sonora": {
				"arquivo": "assets/audio/ambient_techno.mp3",
				"volume": 0.1,
				"loop": true
			},
			"cenas": [
				{
					"id": 1,
					"template": {
						"id": "intro_branding",
						"parametros": {
							"overlay_opacity": 0.3
						}
					},
					"ativos": {
						"media0": {
							"tipo": "imagem",
							"caminho": "assets/img/capa_projeto.jpg"
						}
					},
					"narracao": {
						"texto": "Esta e a cena inicial do nosso teste de integracao de video.",
						"voz": "pt_br"
					}
				},
				{
					"id": 2,
					"template": {
						"id": "cinematic_video",
						"parametros": {
							"blur_amount": 0
						}
					},
					"ativos": {
						"media0": {
							"tipo": "video",
							"caminho": "assets/video/datacenter_drone.mp4"
						}
					},
					"narracao": {
						"texto": "Aqui demonstramos um clipe de video rodando em conjunto com a trilha sonora.",
						"voz": "pt_br"
					}
				}
			]
		}
	}`

	configPath := filepath.Join(tempDir, "config_e2e.json")
	if err := os.WriteFile(configPath, []byte(jsonConfig), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tempDir, "output_e2e.mp4")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Executa a pipeline de geração completa
	err = GenerateVideo(context.Background(), logger, configPath, outputPath, "mock", false)
	if err != nil {
		t.Fatalf("Erro ao executar pipeline E2E de geração de vídeo: %v", err)
	}

	// Verifica se o arquivo de vídeo final foi realmente gerado e tem conteúdo
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Vídeo final não existe: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("Vídeo final gerado possui tamanho zero")
	}

	t.Logf("E2E concluído com sucesso. Vídeo gerado em: %s (%d bytes)", outputPath, info.Size())
}
