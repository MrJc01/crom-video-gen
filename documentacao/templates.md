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

---

## 5. Catálogo de Templates Disponíveis

Aqui estão listados todos os templates nativos configurados no sistema:

### 5.1. `intro_branding`
* **Descrição**: Tela inicial de introdução da marca ou título do projeto com efeito de zoom lento e opacidade ajustável.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Imagem ou vídeo de fundo.
* **Parâmetros**:
  * `text0` (string): Título da introdução.
  * `text1` (string): Subtítulo sutil abaixo.
  * `zoom_speed` (number): Escala de zoom no final da cena (ex: `1.1`).
  * `overlay_opacity` (number): Escuridão de sobreposição (de `0.0` a `1.0`).

### 5.2. `cinematic_video`
* **Descrição**: Exibição de vídeo ou imagem com opções de correção de cor (color grading) e desfoque estilizado.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Vídeo ou imagem principal.
* **Parâmetros**:
  * `color_grading` (string): Estilo de tom do vídeo (`cool_cyan`, `warm_gold`, `vintage_noir`, `cyberpunk_neon`, `teal_orange` ou `default`).
  * `blur_amount` (number): Quantidade de boxblur aplicado (ex: `5`).

### 5.3. `technical_specs`
* **Descrição**: Tópicos ou bullet points aparecendo sobrepostos a uma imagem de fundo (frequentemente diagramas de fluxo).
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Imagem/vídeo de fundo.
* **Parâmetros**:
  * `specs` (array): Lista de strings contendo as especificações técnicas.
  * `list_style` (string): `"bullets"` ou outro estilo de marcador.
  * `highlight_color` (string): Código de cor hexadecimal para realce de texto (ex: `"#ffaa00"`).

### 5.4. `split_screen_demo`
* **Descrição**: Vídeo ou imagem exibido em tela dividida (split-screen) horizontalmente com marcador de divisão móvel.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Mídia exibida na tela.
* **Parâmetros**:
  * `divider_pos` (number): Fração da divisão da tela de `0.0` a `1.0`.
  * `border_width` (number): Espessura da linha divisória.
  * `text0` (string): Legenda do lado esquerdo.
  * `text1` (string): Legenda do lado direito.

### 5.5. `highlight_focus`
* **Descrição**: Escurece e foca a atenção visual do espectador em um ponto específico da mídia de fundo.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Imagem/vídeo principal.
* **Parâmetros**:
  * `focus_area` (string): Posição da área de foco (`top_left`, `top_right`, `bottom_left`, `bottom_right`, `center`).
  * `darken_bg` (boolean): Escurecer ou não as áreas externas à de foco.

### 5.6. `action_video_fast`
* **Descrição**: Execução de um clipe com velocidade acelerada e desfoque de movimento (motion blur) para dinamismo.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Vídeo de ação.
* **Parâmetros**:
  * `playback_speed` (number): Multiplicador de velocidade (ex: `1.5`).
  * `motion_blur` (boolean): Aplicar ou não efeito de blend.

### 5.7. `quote_testimonial`
* **Descrição**: Exibição elegante de citações, depoimentos de clientes ou enunciados de filosofia de equipe.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Imagem de fundo com opacidade atenuada.
* **Parâmetros**:
  * `quote_text` (string): O conteúdo da citação.
  * `quote_author` (string): O autor do depoimento.
  * `accent_color` (string): Cor em formato hexadecimal (ex: `"#00ffcc"`).

### 5.8. `dashboard_kpi`
* **Descrição**: Focado na exibição de indicadores-chave de performance (KPIs) com barra de progresso circular ou linear e valor destacado.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Fundo.
* **Parâmetros**:
  * `metric_value` (string): Valor principal (ex: `"99.98%"`).
  * `metric_label` (string): Descrição do indicador.
  * `progress_percentage` (number): Percentual de progresso na barra (ex: `99.98`).
  * `accent_color` (string): Cor de destaque.

### 5.9. `process_steps`
* **Descrição**: Exibição animada de passos ou sequência lógica de processos de um sistema.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Fundo.
* **Parâmetros**:
  * `steps` (array): Lista de passos em formato string.
  * `accent_color` (string): Cor em hexadecimal.

### 5.10. `outro_credits`
* **Descrição**: Finalização do vídeo com efeito de fade-out completo e inserção de marcação e QR Code.
* **Ativos**:
  * `media0` (Obrigatorio: Sim): Imagem final ou marca.
* **Parâmetros**:
  * `text0` (string): Texto de CTA (ex: `"ACESSE CROM.ME"`).
  * `show_qr_code` (boolean): Se exibe ou não o QR Code de contato.
  * `fade_out_duration` (number): Tempo em segundos para a transição escura no final.

### 5.11. `code_snippet_typing` [NOVO]
* **Descrição**: Simula a digitação de código ou comando de terminal em tempo real. Essencial para documentação técnica e tutoriais.
* **Ativos**:
  * `media0` (Obrigatorio: Não): Mídia de fundo (imagem ou vídeo) com efeito desfocado de ambientação.
* **Parâmetros**:
  * `code_text` (string - Obrigatório): Bloco de texto do código a ser digitado.
  * `language` (string): Linguagem de programação para realce de sintaxe (ex: `"go"`, `"javascript"`, `"json"`, `"python"`).
  * `theme` (string): Tema visual (padrão: `"dracula"`).
  * `typing_speed` (string): Configuração de velocidade.
  * `show_line_numbers` (boolean): Habilitar ou desabilitar numeração de linhas.

### 5.12. `concept_definition` [NOVO]
* **Descrição**: Cartão explicativo focado na definição de um termo, conceito ou jargão corporativo.
* **Ativos**:
  * `media0` (Obrigatorio: Não): Mídia de fundo sob o cartão glassmorphic.
* **Parâmetros**:
  * `term` (string - Obrigatório): O termo a ser definido.
  * `definition` (string - Obrigatório): O bloco de texto contendo a definição.
  * `phonetic_spelling` (string): Pronúncia fonética do termo.
  * `accent_color` (string): Cor de destaque e brilho do cartão (ex: `"#ffaa00"`).

### 5.13. `comparison_matrix` [NOVO]
* **Descrição**: Matriz de comparação lado a lado entre duas abordagens, com realce neon e atenuação do lado perdedor.
* **Ativos**:
  * `media0` (Obrigatorio: Não): Fundo sutil do slide.
* **Parâmetros**:
  * `title` (string - Obrigatório): Título do painel de comparação.
  * `column_a` (object - Obrigatório): Dados da coluna da esquerda. Deve conter:
    * `header` (string): Cabeçalho da coluna.
    * `points` (array): Lista de tópicos textuais.
  * `column_b` (object - Obrigatório): Dados da coluna da direita. Deve conter:
    * `header` (string): Cabeçalho da coluna.
    * `points` (array): Lista de tópicos textuais.
  * `highlight_winner` (string): Identifica o lado vencedor para aplicar o efeito neon (`"column_a"` ou `"column_b"`).

### 5.14. `q_and_a_flashcard` [NOVO]
* **Descrição**: Cartão de perguntas e respostas com visualização 3D. A frente contém a pergunta com um temporizador visual e vira em 180° para revelar a resposta no verso.
* **Ativos**:
  * `media0` (Obrigatorio: Não): Fundo sob a renderização do flashcard.
* **Parâmetros**:
  * `question` (string - Obrigatório): O texto da pergunta.
  * `answer` (string - Obrigatório): O texto da resposta.
  * `flip_animation` (boolean): Ativa o efeito de virada 3D. Se `false`, faz transição de crossfade tradicional.
  * `timer_bar` (number): Duração da barra de progresso.

### 5.15. `roadmap_timeline` [NOVO]
* **Descrição**: Linha do tempo conectada por segmentos para exibição de milestones, fases de entrega ou progresso do projeto.
* **Ativos**:
  * `media0` (Obrigatorio: Não): Fundo sutil da linha do tempo.
* **Parâmetros**:
  * `milestones` (array - Obrigatório): Lista de marcos textuais do roadmap.
  * `current_step_index` (number - Obrigatório): Índice do marco ativo (iniciado em `0`).
  * `orientation` (string): Orientação da linha do tempo (`"horizontal"` ou `"vertical"`).
  * `completed_color` (string): Cor em formato hexadecimal para marcos concluídos (ex: `"#00ffcc"`).
  * `pending_color` (string): Cor em formato hexadecimal para marcos futuros.

```
