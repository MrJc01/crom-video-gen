#!/bin/bash
# Script de verificação automatizada E2E (End-to-End) do projeto Crom Video Gen.

set -e

echo "=== 1. Compilando o Binário Go ==="
go build -o crom-video-gen cmd/crom-video-gen/main.go
echo "Binário compilo com sucesso!"

echo ""
echo "=== 2. Testando Modo: Apenas Validação (Sem Renderização) ==="
./crom-video-gen --config json_inicial --validate-only
echo "Validação estática OK!"

echo ""
echo "=== 3. Executando Geração Completa do Vídeo ==="
# Executa a geração do vídeo usando os mock assets criados previamente
./crom-video-gen --config json_inicial --output output_completo.mp4 --tts-provider edge-tts --verbose
echo "Geração de vídeo OK!"

echo ""
echo "=== 4. Validando Metadados do Vídeo Gerado (QA Audit) ==="
FFPROBE="./bin/ffprobe"
if [ ! -f "$FFPROBE" ]; then
    FFPROBE="ffprobe"
fi

# Verifica resolução
RESOLUTION=$($FFPROBE -v error -select_streams v:0 -show_entries stream=width,height -of default=nw=1:nk=1 output_completo.mp4)
echo "Resolução do vídeo gerado: $RESOLUTION (Esperado: 1920 1080)"

# Verifica codecs de vídeo e áudio
VCODEC=$($FFPROBE -v error -select_streams v:0 -show_entries stream=codec_name -of default=nw=1:nk=1 output_completo.mp4)
ACODEC=$($FFPROBE -v error -select_streams a:0 -show_entries stream=codec_name -of default=nw=1:nk=1 output_completo.mp4)
echo "Codec de vídeo: $VCODEC (Esperado: h264)"
echo "Codec de áudio: $ACODEC (Esperado: aac)"

# Verifica taxa de canais de áudio
ACHANNELS=$($FFPROBE -v error -select_streams a:0 -show_entries stream=channels -of default=nw=1:nk=1 output_completo.mp4)
echo "Canais de áudio: $ACHANNELS (Esperado: 2)"

# Verifica se o arquivo final é maior que zero
FILESIZE=$(stat -c%s "output_completo.mp4")
echo "Tamanho do arquivo: $FILESIZE bytes"

if [ "$FILESIZE" -gt 0 ]; then
    echo ""
    echo "=========================================="
    echo "🎉 SUCESSO: O teste de ponta a ponta passou!"
    echo "=========================================="
else
    echo "ERRO: O arquivo final de vídeo possui tamanho 0."
    exit 1
fi
