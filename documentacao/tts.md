# Motor de Texto-para-Fala (TTS)

O Ecossistema Crom utiliza síntese de voz (Text-to-Speech) para gerar áudios de narração de forma automática. O tempo de duração de cada cena gerada é determinado diretamente pelo tamanho físico em segundos do arquivo de áudio resultante da narração.

---

## 1. Provedores Disponíveis

O sistema suporta dois provedores de TTS principais:

1. **`mock` (Offline / Desenvolvimento):**
   * Não requer conexão com a internet ou dependências locais de IA.
   * Cria arquivos de áudio silenciados (sine wave / beeps) cuja duração em segundos é calculada com base no número de palavras do texto (mínimo de 2 segundos).
   * Ideal para testes rápidos locais, CI ou quando offline.
2. **`edge-tts` (Microsoft Edge Neural Voices):**
   * Utiliza a biblioteca offline/CLI `edge-tts` que se conecta aos servidores públicos de fala do Microsoft Edge.
   * Produz vozes neurais extremamente realistas em português e diversos idiomas.

---

## 2. Configurações de Narração no JSON

Cada cena no arquivo de configuração possui um bloco `"narracao"` onde é possível parametrizar a voz:

```json
"narracao": {
    "texto": "Bem-vindo à apresentação técnica do Ecossistema Crom.",
    "voz": "pt-BR-FranciscaNeural",
    "provedor": "edge-tts",
    "rate": "+5%",
    "pitch": "-2Hz",
    "volume": "-10%"
}
```

### Atributos:
* **`texto`** (Obrigatório): O roteiro que será falado e exibido nas legendas.
* **`voz`** (Opcional): A voz neural a ser usada. Fallbacks automáticos mapeiam chaves simples como `"female"` para `"pt-BR-FranciscaNeural"` ou `"male"` para `"pt-BR-AntonioNeural"`.
* **`provedor`** (Opcional): Permite sobrepor o provedor global especificando `"mock"` ou `"edge-tts"` para aquela cena em particular.
* **`rate`** (Opcional): Velocidade de reprodução da fala (ex: `+10%`, `-5%`).
* **`pitch`** (Opcional): Tom ou frequência da voz (ex: `+5Hz`, `-10%`).
* **`volume`** (Opcional): Volume do áudio gerado (ex: `+5%`, `-10%`).

---

## 3. Instalando o `edge-tts`

Caso deseje utilizar o provedor neural real, garanta que a ferramenta CLI `edge-tts` esteja instalada no sistema operacional.

```bash
pip install edge-tts
```

O resolvedor de executáveis do Crom busca pelo executável `edge-tts` em caminhos do usuário (como `~/.local/bin/edge-tts`) antes de buscá-lo no `PATH` geral do sistema operacional Linux.
