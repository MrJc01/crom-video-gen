package types

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	// Cria arquivo JSON de teste válido
	jsonStr := `{
		"projeto": {
			"titulo": "Projeto Teste",
			"configuracoes_globais": {
				"resolucao": "1920x1080",
				"fps": 30,
				"formato_saida": "mp4",
				"audio": {
					"sample_rate": 48000,
					"bitrate": "192k",
					"canais": 2,
					"codec": "aac",
					"normalizar_volume": true
				}
			},
			"trilha_sonora": {
				"arquivo": "assets/audio/trilha.mp3",
				"volume": 0.15,
				"loop": true
			},
			"cenas": [
				{
					"id": 1,
					"template": {
						"id": "intro_branding",
						"parametros": {}
					},
					"ativos": {
						"media0": {
							"tipo": "imagem",
							"caminho": "assets/img/capa.jpg"
						}
					},
					"narracao": {
						"texto": "Olá mundo",
						"voz": "pt_br"
					}
				}
			]
		}
	}`

	tempFile, err := os.CreateTemp("", "config_test_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write([]byte(jsonStr)); err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	config, err := ParseConfig(tempFile.Name())
	if err != nil {
		t.Fatalf("Erro inesperado ao parsear config válida: %v", err)
	}

	if config.Projeto.Titulo != "Projeto Teste" {
		t.Errorf("Esperado 'Projeto Teste', obtido '%s'", config.Projeto.Titulo)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		errMsg  string
	}{
		{
			name: "titulo vazio",
			jsonStr: `{
				"projeto": {
					"titulo": "",
					"configuracoes_globais": {"resolucao": "1920x1080", "fps": 30, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "aac"}}
				}
			}`,
			errMsg: "titulo não pode ser vazio",
		},
		{
			name: "resolucao invalida",
			jsonStr: `{
				"projeto": {
					"titulo": "Teste",
					"configuracoes_globais": {"resolucao": "1920", "fps": 30, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "aac"}}
				}
			}`,
			errMsg: "resolucao global inválida",
		},
		{
			name: "fps baixo",
			jsonStr: `{
				"projeto": {
					"titulo": "Teste",
					"configuracoes_globais": {"resolucao": "1920x1080", "fps": 10, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "aac"}}
				}
			}`,
			errMsg: "fps global inválido",
		},
		{
			name: "codec nao suportado",
			jsonStr: `{
				"projeto": {
					"titulo": "Teste",
					"configuracoes_globais": {"resolucao": "1920x1080", "fps": 30, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "opus"}}
				}
			}`,
			errMsg: "codec de áudio 'opus' não suportado",
		},
		{
			name: "volume trilha invalido",
			jsonStr: `{
				"projeto": {
					"titulo": "Teste",
					"configuracoes_globais": {"resolucao": "1920x1080", "fps": 30, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "aac"}},
					"trilha_sonora": {"arquivo": "a.mp3", "volume": 1.5, "loop": true}
				}
			}`,
			errMsg: "trilha_sonora.volume deve estar entre 0.0 e 1.0",
		},
		{
			name: "cenas duplicadas",
			jsonStr: `{
				"projeto": {
					"titulo": "Teste",
					"configuracoes_globais": {"resolucao": "1920x1080", "fps": 30, "formato_saida": "mp4", "audio": {"sample_rate": 48000, "bitrate": "192k", "canais": 2, "codec": "aac"}},
					"trilha_sonora": {"arquivo": "a.mp3", "volume": 0.5, "loop": true},
					"cenas": [
						{"id": 1, "template": {"id": "intro_branding"}, "ativos": {"media0": {"tipo": "imagem", "caminho": "a.jpg"}}, "narracao": {"texto": "ola", "voz": "pt"}},
						{"id": 1, "template": {"id": "intro_branding"}, "ativos": {"media0": {"tipo": "imagem", "caminho": "b.jpg"}}, "narracao": {"texto": "mundo", "voz": "pt"}}
					]
				}
			}`,
			errMsg: "cena id 1 duplicada",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config ConfigInput
			err := json.Unmarshal([]byte(tt.jsonStr), &config)
			if err != nil {
				// se falhar o unmarshal mas esperávamos erro de validação
				t.Fatalf("Erro ao decodificar JSON: %v", err)
			}
			err = config.Validate()
			if err == nil {
				t.Fatalf("Esperava erro contendo '%s', mas não obteve erro", tt.errMsg)
			}
			if !testingContains(err.Error(), tt.errMsg) {
				t.Errorf("Esperava erro contendo '%s', obteve '%v'", tt.errMsg, err)
			}
		})
	}
}

func testingContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s[0:len(substr)] == substr || len(s) > len(substr) && testingContains(s[1:], substr))
}
