package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"crom-video-gen/internal/execs"
)

var (
	resolutionRegex = regexp.MustCompile(`^(\d+)x(\d+)$`)
	validCodecs     = map[string]bool{"aac": true, "mp3": true, "libmp3lame": true}
	validAudioRates = map[int]bool{22050: true, 32000: true, 44100: true, 48000: true}
	validTemplates  = map[string]bool{
		"intro_branding":     true,
		"cinematic_video":    true,
		"technical_specs":    true,
		"split_screen_demo":  true,
		"highlight_focus":    true,
		"action_video_fast":  true,
		"outro_credits":      true,
	}
	rateRegex   = regexp.MustCompile(`^[+-]\d+%$`)
	pitchRegex  = regexp.MustCompile(`^[+-]\d+(Hz|%)?$`)
	volumeRegex = regexp.MustCompile(`^[+-]\d+%$`)
)

type AtivoSchema struct {
	Obrigatorio     bool     `json:"obrigatorio"`
	TiposPermitidos []string `json:"tipos_permitidos"`
}

type ParametroSchema struct {
	Obrigatorio bool   `json:"obrigatorio"`
	Tipo        string `json:"tipo"`
}

type TemplateSchema struct {
	Ativos     map[string]AtivoSchema     `json:"ativos"`
	Parametros map[string]ParametroSchema `json:"parametros"`
}

// Validate valida a integridade lógica da struct ConfigInput e os schemas dos templates
func (c *ConfigInput) Validate() error {
	p := c.Projeto

	if strings.TrimSpace(p.Titulo) == "" {
		return fmt.Errorf("projeto.titulo não pode ser vazio")
	}

	// Validação de Configurações Globais
	g := p.ConfiguracoesGlobais
	if !resolutionRegex.MatchString(g.Resolucao) {
		return fmt.Errorf("resolucao global inválida: '%s' (deve ser no formato LARGURAxALTURA, ex: 1920x1080)", g.Resolucao)
	}

	if g.FPS < 15 || g.FPS > 120 {
		return fmt.Errorf("fps global inválido: %d (deve estar entre 15 e 120)", g.FPS)
	}

	if g.FormatoSaida != "mp4" && g.FormatoSaida != "mkv" {
		return fmt.Errorf("formato_saida '%s' não suportado (apenas 'mp4' e 'mkv' são permitidos)", g.FormatoSaida)
	}

	// Validação de Áudio
	a := g.Audio
	if !validCodecs[strings.ToLower(a.Codec)] {
		return fmt.Errorf("codec de áudio '%s' não suportado (apenas 'aac' e 'mp3' são permitidos)", a.Codec)
	}

	if !validAudioRates[a.SampleRate] {
		return fmt.Errorf("sample_rate de áudio %d não suportado (use 22050, 32000, 44100 ou 48000)", a.SampleRate)
	}

	if a.Canais != 1 && a.Canais != 2 {
		return fmt.Errorf("canais de áudio %d inválido (apenas 1 ou 2 canais são permitidos)", a.Canais)
	}

	// Validação de Trilha Sonora
	t := p.TrilhaSonora
	if strings.TrimSpace(t.Arquivo) == "" {
		return fmt.Errorf("trilha_sonora.arquivo não pode ser vazio")
	}
	if t.Volume < 0.0 || t.Volume > 1.0 {
		return fmt.Errorf("trilha_sonora.volume deve estar entre 0.0 e 1.0 (recebido: %f)", t.Volume)
	}

	// Validação das Cenas
	if len(p.Cenas) == 0 {
		return fmt.Errorf("o projeto deve conter pelo menos uma cena")
	}

	sceneIDs := make(map[int]bool)
	projectRoot := execs.FindProjectRoot()

	for i, cena := range p.Cenas {
		// IDs de cena duplicados
		if sceneIDs[cena.ID] {
			return fmt.Errorf("cena id %d duplicada no JSON", cena.ID)
		}
		sceneIDs[cena.ID] = true

		// Validar templates
		if !validTemplates[cena.Template.ID] {
			return fmt.Errorf("cena %d (índice %d) possui template desconhecido: '%s'", cena.ID, i, cena.Template.ID)
		}

		// Carregar e validar contra o schema.json do template
		schemaPath := filepath.Join(projectRoot, "templates", cena.Template.ID, "schema.json")
		schemaData, err := os.ReadFile(schemaPath)
		if err != nil {
			return fmt.Errorf("cena %d (índice %d) erro ao carregar schema do template '%s': %w", cena.ID, i, cena.Template.ID, err)
		}

		var schema TemplateSchema
		if err := json.Unmarshal(schemaData, &schema); err != nil {
			return fmt.Errorf("cena %d (índice %d) erro no parsing de schema do template '%s': %w", cena.ID, i, cena.Template.ID, err)
		}

		// 1. Validar os ativos informados contra o schema
		for schemaKey, valSchema := range schema.Ativos {
			cenaAtivo, ok := cena.Ativos[schemaKey]
			if !ok {
				if valSchema.Obrigatorio {
					return fmt.Errorf("cena %d (índice %d) ativo obrigatório '%s' ausente para o template '%s'", cena.ID, i, schemaKey, cena.Template.ID)
				}
				continue
			}

			// Validar tipo do ativo
			tipoValido := false
			for _, t := range valSchema.TiposPermitidos {
				if cenaAtivo.Tipo == t {
					tipoValido = true
					break
				}
			}
			if !tipoValido {
				return fmt.Errorf("cena %d (índice %d) ativo '%s' possui tipo inválido '%s' (permitidos: %v)", cena.ID, i, schemaKey, cenaAtivo.Tipo, valSchema.TiposPermitidos)
			}

			// Validar caminho
			if strings.TrimSpace(cenaAtivo.Caminho) == "" {
				return fmt.Errorf("cena %d (índice %d) ativo '%s' possui caminho vazio", cena.ID, i, schemaKey)
			}
		}

		// Validar se existem ativos extras não mapeados no schema ou com caminhos vazios
		for key, cenaAtivo := range cena.Ativos {
			if _, ok := schema.Ativos[key]; !ok {
				return fmt.Errorf("cena %d (índice %d) possui ativo extra '%s' não definido no schema do template '%s'", cena.ID, i, key, cena.Template.ID)
			}
			if cenaAtivo.Tipo != "imagem" && cenaAtivo.Tipo != "video" {
				return fmt.Errorf("cena %d (índice %d) ativo '%s' possui tipo inválido: '%s' (deve ser 'imagem' ou 'video')", cena.ID, i, key, cenaAtivo.Tipo)
			}
			if strings.TrimSpace(cenaAtivo.Caminho) == "" {
				return fmt.Errorf("cena %d (índice %d) ativo '%s' possui caminho vazio", cena.ID, i, key)
			}
		}

		// 2. Validar os parametros informados contra o schema
		for schemaKey, valSchema := range schema.Parametros {
			paramVal, ok := cena.Template.Parametros[schemaKey]
			if !ok {
				if valSchema.Obrigatorio {
					return fmt.Errorf("cena %d (índice %d) parâmetro obrigatório '%s' ausente para o template '%s'", cena.ID, i, schemaKey, cena.Template.ID)
				}
				continue
			}

			// Validar tipo do parâmetro
			switch valSchema.Tipo {
			case "number":
				_, okNumber := paramVal.(float64)
				if !okNumber {
					return fmt.Errorf("cena %d (índice %d) parâmetro '%s' deve ser do tipo número (recebido: %T)", cena.ID, i, schemaKey, paramVal)
				}
			case "string":
				_, okStr := paramVal.(string)
				if !okStr {
					return fmt.Errorf("cena %d (índice %d) parâmetro '%s' deve ser do tipo string (recebido: %T)", cena.ID, i, schemaKey, paramVal)
				}
			case "boolean":
				_, okBool := paramVal.(bool)
				if !okBool {
					return fmt.Errorf("cena %d (índice %d) parâmetro '%s' deve ser do tipo booleano (recebido: %T)", cena.ID, i, schemaKey, paramVal)
				}
			case "array":
				_, okSlice := paramVal.([]interface{})
				if !okSlice {
					return fmt.Errorf("cena %d (índice %d) parâmetro '%s' deve ser do tipo array (recebido: %T)", cena.ID, i, schemaKey, paramVal)
				}
			}
		}

		// Validar narração
		if strings.TrimSpace(cena.Narracao.Texto) == "" {
			return fmt.Errorf("cena %d (índice %d) possui narracao.texto vazia", cena.ID, i)
		}

		prov := strings.ToLower(cena.Narracao.Provedor)
		if prov != "" && prov != "mock" && prov != "edge-tts" {
			return fmt.Errorf("cena %d (índice %d) possui provedor de narração inválido: '%s' (apenas 'mock' ou 'edge-tts' são permitidos)", cena.ID, i, cena.Narracao.Provedor)
		}

		if cena.Narracao.Rate != "" && !rateRegex.MatchString(cena.Narracao.Rate) {
			return fmt.Errorf("cena %d (índice %d) possui rate de narração inválido: '%s' (deve ser no formato [+-]Valor%%, ex: +10%% ou -5%%)", cena.ID, i, cena.Narracao.Rate)
		}

		if cena.Narracao.Pitch != "" && !pitchRegex.MatchString(cena.Narracao.Pitch) {
			return fmt.Errorf("cena %d (índice %d) possui pitch de narração inválido: '%s' (deve ser no formato [+-]ValorHz ou [+-]Valor%%, ex: +5Hz ou -10%%)", cena.ID, i, cena.Narracao.Pitch)
		}

		if cena.Narracao.Volume != "" && !volumeRegex.MatchString(cena.Narracao.Volume) {
			return fmt.Errorf("cena %d (índice %d) possui volume de narração inválido: '%s' (deve ser no formato [+-]Valor%%, ex: +10%% ou -10%%)", cena.ID, i, cena.Narracao.Volume)
		}
	}

	return nil
}

// GetResolutionWidthAndHeight parses resolution string like "1920x1080" into ints
func (g *GlobalConfig) GetResolutionWidthAndHeight() (int, int, error) {
	matches := resolutionRegex.FindStringSubmatch(g.Resolucao)
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("resolução inválida: %s", g.Resolucao)
	}
	w, _ := strconv.Atoi(matches[1])
	h, _ := strconv.Atoi(matches[2])
	return w, h, nil
}
