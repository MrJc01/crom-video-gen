# Templates de Cena, Variáveis Dinâmicas e Validação por Schema

Os templates de cena no Crom são desenvolvidos utilizando tecnologias padrão da web: **HTML, CSS e JavaScript**. Isso permite criar designs sofisticados, tipografia avançada e animações ricas que são renderizadas de forma determinística por um navegador headless.

---

## 1. Estrutura de um Template

Cada template reside em uma subpasta dentro do diretório `/templates/`. O nome da pasta corresponde ao ID do template utilizado no arquivo de configuração JSON. 

A estrutura de arquivos recomendada é:
```text
/templates/nome_do_template/
├── schema.json          # Regras de validação estática de ativos e parâmetros
└── index.html           # Marcação, estilização CSS e lógica JavaScript
```

---

## 2. Definindo a Especificação (`schema.json`)

O arquivo `schema.json` serve para definir quais ativos e parâmetros opcionais ou obrigatórios o template aceita. O motor Go carrega e valida o arquivo de configuração do usuário contra estas especificações antes de iniciar a compilação do vídeo.

### Exemplo de Estrutura de `schema.json`:
```json
{
  "ativos": {
    "media0": { "obrigatorio": true, "tipos_permitidos": ["imagem", "video"] },
    "media1": { "obrigatorio": false, "tipos_permitidos": ["imagem", "video"] }
  },
  "parametros": {
    "divider_pos": { "obrigatorio": false, "tipo": "number" },
    "text0": { "obrigatorio": false, "tipo": "string" },
    "specs": { "obrigatorio": false, "tipo": "array" }
  }
}
```

### Campos do Schema:
* **`ativos`**: Mapeia as chaves das mídias. Cada item possui:
  * `obrigatorio` (bool): Determina se o ativo precisa ser informado.
  * `tipos_permitidos` (array): Quais extensões/tipos são suportados (`"imagem"` ou `"video"`).
* **`parametros`**: Mapeia as variáveis adicionais enviadas no dicionário `parametros`. Cada item possui:
  * `obrigatorio` (bool): Determina se a variável precisa ser informada.
  * `tipo` (string): O tipo do dado esperado (`"string"`, `"number"`, `"boolean"`, `"array"`).

---

## 3. Desenvolvendo a Lógica do Template (`index.html`)

O arquivo `index.html` deve expor duas funções globais no objeto `window`:

### 3.1. `window.setupTemplate(cenaJson)`
Invocada no início do processamento da cena. Ela recebe os dados estruturados da cena em formato JSON. Nesta função, você deve:
* Injetar textos de legenda no contêiner adequado (`cena.narracao.texto`).
* Criar e adicionar elementos `<img>` ou `<video>` com base em `cena.ativos.media0.caminho` (e mídias adicionais se aplicável).
* Configurar variáveis dinâmicas de estilo, cores ou textos alternativos lidos de `cena.template.parametros`.

> [!IMPORTANT]
> Se o ativo for um vídeo, defina `mediaElement.muted = true`. O Headless Chrome exige que vídeos estejam silenciados para que seja permitido controlar o andamento do vídeo e disparar buscas de frames (`seek`).

### 3.2. `window.seekTo(timeInSeconds, totalDuration)`
Invocada deterministicamente frame-a-frame no loop de captura do Go.
* **Mídias de Vídeo:** Você deve buscar o frame correto atribuindo `mediaElement.currentTime = timeInSeconds`.
* **Animações e Efeitos JS:** O avanço de animações deve ser linearmente calculado a partir do progresso (`timeInSeconds / totalDuration`), garantindo estabilidade e impedindo distorções causadas por lag da CPU (wall-clock time).

---

## 4. Exemplo Prático de Código (`index.html`)

Abaixo está um modelo simplificado de template dinâmico que renderiza um texto customizado e uma imagem de fundo:

```html
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <link rel="stylesheet" href="/templates/common/base.css">
    <style>
        .title-overlay {
            position: absolute;
            top: 40%;
            left: 50%;
            transform: translate(-50%, -50%);
            font-size: 48px;
            color: white;
            z-index: 10;
        }
    </style>
</head>
<body>
    <div id="viewport">
        <div class="media-container" id="media-box"></div>
        <div class="title-overlay" id="title-box"></div>
        <div class="subtitles-container">
            <div id="subtitles" class="subtitles-text"></div>
        </div>
    </div>

    <script>
        let bgImg = null;

        window.setupTemplate = function(cenaJson) {
            const cena = JSON.parse(cenaJson);
            
            // Define legenda de narração
            document.getElementById('subtitles').innerText = cena.narracao.texto;
            
            // Define imagem de fundo
            const mediaBox = document.getElementById('media-box');
            mediaBox.innerHTML = '';
            if (cena.ativos && cena.ativos.media0) {
                bgImg = document.createElement('img');
                bgImg.src = cena.ativos.media0.caminho;
                mediaBox.appendChild(bgImg);
            }

            // Define título dinâmico (text0)
            const params = cena.template.parametros || {};
            const titleBox = document.getElementById('title-box');
            if (params.text0) {
                titleBox.innerText = params.text0;
            }
        };

        window.seekTo = function(timeInSeconds, totalDuration) {
            // Efeitos e transformações calculados linearmente
            if (bgImg) {
                const progress = timeInSeconds / totalDuration;
                const scale = 1.0 + (0.1 * progress);
                bgImg.style.transform = `scale(${scale})`;
            }
        };
    </script>
</body>
</html>
```
