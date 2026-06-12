# Configuração do Projeto (JSON de Entrada)

A montagem do vídeo final é guiada por um arquivo JSON. Este documento explica a estrutura de dados e as opções disponíveis para controle da pipeline.

---

## 1. Estrutura Completa do JSON

O JSON de configuração segue o seguinte modelo hierárquico:

```json
{
  "projeto": {
    "titulo": "Nome do Vídeo",
    "configuracoes_globais": {
      "resolucao": "1920x1080",
      "fps": 30,
      "formato_saida": "mp4",
      "audio": {
        "sample_rate": 48000,
        "bitrate": "320k",
        "canais": 2,
        "codec": "aac",
        "normalizar_volume": true
      }
    },
    "trilha_sonora": {
      "arquivo": "assets/audio/ambient_techno.mp3",
      "volume": 0.12,
      "loop": true
    },
    "cenas": [
      {
        "id": 1,
        "template": {
          "id": "intro_branding",
          "parametros": {
            "zoom_speed": 1.1,
            "overlay_opacity": 0.4,
            "text0": "Ecossistema Crom"
          }
        },
        "ativos": {
          "media0": {
            "tipo": "imagem",
            "caminho": "assets/img/capa_projeto.jpg"
          }
        },
        "narracao": {
          "texto": "Bem-vindo à apresentação técnica.",
          "voz": "pt-BR-FranciscaNeural"
        }
      }
    ]
  }
}
```

---

## 2. Detalhes das Chaves de Configuração

### 2.1. Configurações Globais (`configuracoes_globais`)
* **`resolucao`** (string): Resolução do vídeo final no formato `LARGURAxALTURA`. Ex: `"1920x1080"` ou `"1280x720"`.
* **`fps`** (int): Quadros por segundo (FPS). Valores permitidos entre `15` e `120`.
* **`formato_saida`** (string): Extensão do arquivo final (`"mp4"` ou `"mkv"`).
* **`audio`**: Bloco de controle da qualidade de áudio:
  * `sample_rate`: Frequência de amostragem (`22050`, `32000`, `44100` ou `48000`).
  * `bitrate`: Bitrate de codificação (ex: `"192k"`, `"320k"`).
  * `canais`: Canais de áudio (`1` para Mono, `2` para Estéreo).
  * `codec`: Codec do áudio (`"aac"` ou `"mp3"`).
  * `normalizar_volume` (bool): Ativa o filtro `loudnorm` do FFmpeg na mixagem final.

### 2.2. Trilha Sonora (`trilha_sonora`)
* **`arquivo`** (string): Caminho físico para o áudio de fundo (ex: mp3/wav).
* **`volume`** (float): Volume da trilha de fundo, de `0.0` (mudo) a `1.0` (máximo). Ex: `0.12`.
* **`loop`** (bool): Repetir a música indefinidamente até o fim do vídeo.

### 2.3. Cenas (`cenas`)
* **`id`** (int): Identificador único numérico sequencial da cena.
* **`template`**: Parâmetros visuais:
  * `id`: O ID correspondente à pasta em `/templates/`.
  * `parametros`: Mapa chave-valor contendo parâmetros do template e variáveis de texto.
* **`ativos`**: Mapa de mídias de entrada necessárias para a cena (ex: `"media0"`, `"media1"`). Cada ativo possui:
  * `tipo`: `"imagem"` ou `"video"`.
  * `caminho`: Caminho para o arquivo físico.
* **`narracao`**: Bloco de texto e sintetização TTS para a cena (detalhado no documento de TTS).
