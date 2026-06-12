# Crom Video Gen: Motor de Vídeo Baseado em HTML/CSS/JS

O **Crom Video Gen** é um gerador de vídeos automatizado escrito em **Go** que interpreta especificações em JSON e renderiza de forma determinística cenas dinâmicas e animações ricas em **HTML/CSS/JS** utilizando **Headless Chrome** (`chromedp`) e **FFmpeg**.

---

## 🚀 Recursos
* **Modelagem Web:** Crie e edite templates usando HTML, CSS, JavaScript, WebGL ou SVG.
* **Validação por Schema:** Cada template possui um arquivo `schema.json` local. O motor Go audita a consistência de mídias e parâmetros dinâmicos (como strings, números, booleanos ou arrays) antes da renderização.
* **Suporte a Múltiplos Ativos:** Passe mapas de mídias e ativos por cena (ex: `media0`, `media1`), permitindo montagens lado a lado ou vinhetas complexas.
* **Sincronização Frame-a-Frame:** O andamento é controlado deterministicamente no tempo do vídeo (independente da velocidade de CPU/FPS), aguardando eventos `seeked` de tags de vídeo para evitar jitter e telas pretas.
* **Narração via Edge-TTS:** Integração nativa com a API de voz neural do Microsoft Edge para produzir falas humanas com controle de velocidade, tom e volume.
* **Mjpeg Streaming (Fast I/O):** Transmite os frames capturados via memória diretamente no `stdin` do FFmpeg, evitando escrita de arquivos de imagens temporários no disco.

---

## 🛠️ Pré-requisitos
Para executar o projeto localmente, certifique-se de possuir:
1. **Go 1.22.x** ou superior.
2. **Google Chrome** ou **Chromium** instalado (localizado em `/usr/bin/google-chrome` ou no PATH).
3. **FFmpeg** e **FFprobe** instalados (ou binários locais em `./bin/`).
4. **edge-tts** (instalável via `pip install edge-tts` para síntese neural real).

---

## 📂 Organização do Repositório
* **`cmd/crom-video-gen/`**: Ponto de entrada do CLI executável.
* **`pkg/types/`**: Definição de structs, parser de JSON e validação contra `schema.json`.
* **`pkg/render/`**: Core do motor de renderização. Inicializa o servidor HTTP local e o loop do `chromedp`.
* **`pkg/tts/`**: Adaptadores para síntese de voz (`mock` offline e `edge-tts` em nuvem).
* **`templates/`**: Pasta contendo os templates dinâmicos HTML/CSS/JS e suas especificações de validação.
* **`documentacao/`**: Pasta com guias detalhados de arquitetura, templates e TTS.

---

## ⚙️ Como Compilar e Executar

### 1. Compilar o Binário
```bash
go build -o crom-video-gen cmd/crom-video-gen/main.go
```

### 2. Flags do CLI
* `--config` (padrão: `json_inicial`): Caminho do arquivo JSON de configuração.
* `--output` (padrão: `output.mp4`): Caminho para salvar o vídeo gerado.
* `--tts-provider` (padrão: `edge-tts`): Seleção do provedor de TTS (`mock` ou `edge-tts`).
* `--validate-only`: Apenas executa a validação de schemas e ativos sem renderizar.
* `--verbose`: Ativa logs detalhados de nível `debug`.

### 3. Exemplo Prático de Geração
```bash
./crom-video-gen --config json_inicial --output output_completo.mp4 --verbose
```

---

## 📁 Documentação Detalhada
Para guias completos sobre o funcionamento do ecossistema, consulte a pasta `/documentacao/`:
* **[Arquitetura do Sistema](file:///home/j/Documentos/GitHub/crom-test1/documentacao/arquitetura.md)**
* **[Templates e Variáveis Dinâmicas](file:///home/j/Documentos/GitHub/crom-test1/documentacao/templates.md)**
* **[Sintetizador TTS](file:///home/j/Documentos/GitHub/crom-test1/documentacao/tts.md)**
* **[Esquema de Configuração JSON](file:///home/j/Documentos/GitHub/crom-test1/documentacao/configuracao.md)**
