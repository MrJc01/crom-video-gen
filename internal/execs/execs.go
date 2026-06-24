package execs

import (
	"os"
	"os/exec"
	"path/filepath"
)

// FindProjectRoot tenta encontrar a pasta raiz do projeto (onde go.mod existe)
func FindProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}

	for {
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

