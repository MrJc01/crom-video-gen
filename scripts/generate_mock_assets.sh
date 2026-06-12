#!/bin/bash
# Script para gerar ativos mockados e válidos usando FFmpeg para teste E2E do gerador de vídeo.

set -e

# Garante que o script roda a partir da raiz do projeto
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$( dirname "$SCRIPT_DIR" )"
cd "$PROJECT_ROOT"

FFMPEG="./bin/ffmpeg"
if [ ! -f "$FFMPEG" ]; then
    FFMPEG="ffmpeg"
fi

echo "Iniciando geração de ativos mock de mídia..."

# Cria pastas se não existirem
mkdir -p assets/audio assets/img assets/video

# 1. Trilha sonora (.mp3) - gera um tom de áudio senoidal leve de 440Hz com 60 segundos
echo "Gerando trilha sonora mock..."
$FFMPEG -y -f lavfi -i "sine=frequency=220:sample_rate=48000" -t 60 -c:a libmp3lame -b:a 128k assets/audio/ambient_techno.mp3

# 2. Imagens (.jpg e .png)
echo "Gerando imagens de capa e diagramas..."
$FFMPEG -y -f lavfi -i color=c=0x1a1a2e:s=1920x1080:d=1 -vframes 1 assets/img/capa_projeto.jpg
$FFMPEG -y -f lavfi -i color=c=0x16213e:s=1920x1080:d=1 -vframes 1 assets/img/diagrama_fluxo.png
$FFMPEG -y -f lavfi -i color=c=0x0f3460:s=1920x1080:d=1 -vframes 1 assets/img/modulo_hardware.jpg
$FFMPEG -y -f lavfi -i color=c=0x1a1a2e:s=1920x1080:d=1 -vframes 1 assets/img/final_branding.jpg

# 3. Vídeos (.mp4) - gera vídeos usando a fonte de testes testsrc do ffmpeg
echo "Gerando clipes de vídeo mock..."
$FFMPEG -y -f lavfi -i testsrc=size=1920x1080:rate=30 -t 15 -c:v libx264 -pix_fmt yuv420p assets/video/datacenter_drone.mp4
$FFMPEG -y -f lavfi -i testsrc=size=1920x1080:rate=30 -t 15 -c:v libx264 -pix_fmt yuv420p assets/video/cli_interface_demo.mp4
$FFMPEG -y -f lavfi -i testsrc=size=1920x1080:rate=30 -t 15 -c:v libx264 -pix_fmt yuv420p assets/video/network_propagation.mp4

echo "Todos os ativos mock foram criados com sucesso na pasta assets/"
ls -la assets/audio assets/img assets/video
