package types

type AudioConfig struct {
	SampleRate       int    `json:"sample_rate"`
	Bitrate          string `json:"bitrate"`
	Canais           int    `json:"canais"`
	Codec            string `json:"codec"`
	NormalizarVolume bool   `json:"normalizar_volume"`
}

type GlobalConfig struct {
	Resolucao    string      `json:"resolucao"`
	FPS          int         `json:"fps"`
	FormatoSaida string      `json:"formato_saida"`
	Audio        AudioConfig `json:"audio"`
}

type TrilhaSonora struct {
	Arquivo string  `json:"arquivo"`
	Volume  float64 `json:"volume"`
	Loop    bool    `json:"loop"`
}

type Template struct {
	ID         string                 `json:"id"`
	Parametros map[string]interface{} `json:"parametros"`
}

type Ativo struct {
	Tipo    string `json:"tipo"`
	Caminho string `json:"caminho"`
}

type Narracao struct {
	Texto    string `json:"texto"`
	Voz      string `json:"voz"`
	Provedor string `json:"provedor,omitempty"`
	Rate     string `json:"rate,omitempty"`
	Pitch    string `json:"pitch,omitempty"`
	Volume   string `json:"volume,omitempty"`
}

type Cena struct {
	ID       int              `json:"id"`
	Template Template         `json:"template"`
	Ativos   map[string]Ativo `json:"ativos"`
	Narracao Narracao         `json:"narracao"`
}

type Projeto struct {
	Titulo               string       `json:"titulo"`
	ConfiguracoesGlobais GlobalConfig `json:"configuracoes_globais"`
	TrilhaSonora         TrilhaSonora `json:"trilha_sonora"`
	Cenas                []Cena       `json:"cenas"`
}

type ConfigInput struct {
	Projeto Projeto `json:"projeto"`
}
