#!/usr/bin/env python3
import subprocess
import json
import time
import sys
from datetime import datetime

def run_command(cmd, shell=False):
    print(f"Executing: {' '.join(cmd) if isinstance(cmd, list) else cmd}")
    result = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, shell=shell)
    return result

def main():
    print("=== 1. Compilando o Binário Go ===")
    build_res = run_command(["go", "build", "-o", "crom-video-gen", "cmd/crom-video-gen/main.go"])
    if build_res.returncode != 0:
        print("Erro de compilação:")
        print(build_res.stderr)
        sys.exit(1)
    print("Compilado com sucesso.\n")

    print("=== 2. Gerando Ativos Mock se necessário ===")
    # Executa a geração de mock assets para garantir que existam
    run_command(["bash", "scripts/generate_mock_assets.sh"])
    print("Ativos mock preparados.\n")

    print("=== 3. Executando Geração e Coletando Métricas ===")
    cmd = [
        "./crom-video-gen",
        "--config", "json_inicial",
        "--output", "output_benchmark.mp4",
        "--tts-provider", "edge-tts",
        "--log-format", "json",
        "--verbose"
    ]
    
    start_real = time.time()
    # Roda o gerador e coleta o output do stdout (onde os logs do slog são direcionados)
    process = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    
    log_lines = []
    # Lê linha a linha em tempo real
    while True:
        line = process.stdout.readline()
        if not line and process.poll() is not None:
            break
        if line:
            print(line.strip()) # Imprime no console também para o usuário acompanhar
            log_lines.append(line.strip())

    stderr_output = process.stderr.read()
    if stderr_output:
        print("Stderr extra:")
        print(stderr_output)
        
    end_real = time.time()
    total_real = end_real - start_real

    print("\n" + "="*50)
    print("   RELATÓRIO DE BENCHMARK DE PERFORMANCE")
    print("="*50)

    parsed_logs = []
    for line in log_lines:
        try:
            parsed_logs.append(json.loads(line))
        except json.JSONDecodeError:
            # Ignora linhas que não são JSON (ex: pânicos ou outputs diretos do ffmpeg)
            continue

    if not parsed_logs:
        print("Nenhum log estruturado JSON foi capturado.")
        sys.exit(1)

    # Função auxiliar para parsear timestamp
    def parse_time(ts_str):
        # Exemplo: 2026-06-12T03:52:15.123-03:00 ou similar
        # Vamos remover o timezone para parse simples se necessário ou usar deiso8601
        # Python 3.11+ suporta ISO com timezone usando datetime.fromisoformat
        try:
            return datetime.fromisoformat(ts_str)
        except Exception:
            # Fallback removendo milissegundos ou timezone se der erro
            cleaned = ts_str.split("-")[0].split("+")[0]
            return datetime.strptime(cleaned, "%Y-%m-%dT%H:%M:%S")

    t_start = None
    t_validation_end = None
    scenes = {} # cena_id -> {start, tts, end}
    t_mix_start = None
    t_end = None
    last_tts_time = None

    for log in parsed_logs:
        msg = log.get("msg", "")
        ts = parse_time(log.get("time"))
        cena_id = log.get("cena_id")

        if cena_id is not None and cena_id not in scenes:
            scenes[cena_id] = {}

        if "Iniciando processo de geração de vídeo" in msg:
            t_start = ts
        elif "Todos os ativos físicos foram localizados e validados com sucesso" in msg:
            t_validation_end = ts
            last_tts_time = ts
        elif "Áudio de narração gerado" in msg:
            if cena_id is not None and last_tts_time is not None:
                scenes[cena_id]["tts_duration"] = (ts - last_tts_time).total_seconds()
                last_tts_time = ts
        elif "Processando cena" in msg:
            if cena_id is not None:
                scenes[cena_id]["render_start"] = ts
        elif "Cena renderizada com sucesso" in msg:
            if cena_id is not None:
                scenes[cena_id]["render_end"] = ts
        elif "Concatenando cenas" in msg:
            t_mix_start = ts
        elif "Processo de geração de vídeo finalizado com sucesso" in msg:
            t_end = ts

    # Print do relatório detalhado
    if t_start and t_validation_end:
        val_time = (t_validation_end - t_start).total_seconds()
        print(f"1. Inicialização & Validação: {val_time:.3f} s")
    else:
        print("1. Inicialização & Validação: N/A")

    print("\n2. Processamento por Cena:")
    for cid, sdata in sorted(scenes.items()):
        tts_dur = sdata.get("tts_duration", 0.0)
        r_start = sdata.get("render_start")
        r_end = sdata.get("render_end")

        print(f"   - Cena {cid}:")
        print(f"     * TTS (edge-tts): {tts_dur:.3f} s")
        if r_start and r_end:
            render_dur = (r_end - r_start).total_seconds()
            print(f"     * Render (Chrome + FFmpeg): {render_dur:.3f} s")
            print(f"     * Total Cena: {(tts_dur + render_dur):.3f} s")
        else:
            print(f"     * Render (Chrome + FFmpeg): N/A")

    if t_mix_start and t_end:
        mix_time = (t_end - t_mix_start).total_seconds()
        print(f"\n3. Concatenação e Mixagem Final (FFmpeg): {mix_time:.3f} s")
    else:
        print("\n3. Concatenação e Mixagem Final (FFmpeg): N/A")

    print("\n" + "-"*50)
    print(f"Tempo total via logs: {(t_end - t_start).total_seconds():.3f} s" if t_start and t_end else "Tempo total via logs: N/A")
    print(f"Tempo total real (Wall Clock): {total_real:.3f} s")
    print("="*50)

if __name__ == "__main__":
    main()
