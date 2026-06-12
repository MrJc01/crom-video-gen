package execs

import (
	"os"
	"os/exec"
	"path/filepath"
)

// FindProjectRoot tenta encontrar a pasta raiz do projeto (onde go.mod ou bin/ffmpeg existe)
func FindProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
		// Verifica se bin/ffmpeg existe nesta pasta
		localPath := filepath.Join(dir, "bin", "ffmpeg")
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			return dir
		}

		// Verifica se go.mod existe nesta pasta
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // chegou na raiz do sistema de arquivos
		}
		dir = parent
	}
	return "."
}

// ResolveFFmpegPath tenta encontrar o executável do ffmpeg local ou no sistema
func ResolveFFmpegPath() string {
	// 1. Tenta na pasta bin/ local a partir da raiz do projeto
	root := FindProjectRoot()
	localPath := filepath.Join(root, "bin", "ffmpeg")
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return localPath
	}

	// 2. Tenta a partir do executável atual
	if exePath, err := os.Executable(); err == nil {
		localPathExe := filepath.Join(filepath.Dir(exePath), "bin", "ffmpeg")
		if info, err := os.Stat(localPathExe); err == nil && !info.IsDir() {
			return localPathExe
		}
	}

	// 3. Fallback para o PATH do sistema
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return "ffmpeg"
}

// ResolveFFprobePath tenta encontrar o executável do ffprobe local ou no sistema
func ResolveFFprobePath() string {
	// 1. Tenta na pasta bin/ local a partir da raiz do projeto
	root := FindProjectRoot()
	localPath := filepath.Join(root, "bin", "ffprobe")
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return localPath
	}

	// 2. Tenta a partir do executável atual
	if exePath, err := os.Executable(); err == nil {
		localPathExe := filepath.Join(filepath.Dir(exePath), "bin", "ffprobe")
		if info, err := os.Stat(localPathExe); err == nil && !info.IsDir() {
			return localPathExe
		}
	}

	// 3. Fallback para o PATH do sistema
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return "ffprobe"
}

// ResolveEdgeTTSPath tenta encontrar o executável do edge-tts local ou no sistema
func ResolveEdgeTTSPath() string {
	// 1. Tenta no diretório home do usuário ~/.local/bin/edge-tts
	if home, err := os.UserHomeDir(); err == nil {
		localPath := filepath.Join(home, ".local", "bin", "edge-tts")
		if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
			return localPath
		}
	}

	// 2. Fallback para o PATH do sistema
	if p, err := exec.LookPath("edge-tts"); err == nil {
		return p
	}
	return "edge-tts"
}
