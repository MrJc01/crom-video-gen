package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AssetManager lida com a validação física dos ativos em disco e criação de temporários
type AssetManager struct {
	baseDir string
	tempDir string
}

// NewAssetManager cria um novo AssetManager
func NewAssetManager(baseDir string) *AssetManager {
	return &AssetManager{
		baseDir: filepath.Clean(baseDir),
	}
}

// SanitizePath sanitiza o caminho do arquivo para evitar Directory Traversal
func (am *AssetManager) SanitizePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(cleanPath) {
		return cleanPath, nil
	}
	// Se for relativo, resolve com base no baseDir
	resolved := filepath.Join(am.baseDir, cleanPath)
	return resolved, nil
}

// ValidateFile verifica se o arquivo existe e se é regular
func (am *AssetManager) ValidateFile(path string, allowedExtensions []string) error {
	resolvedPath, err := am.SanitizePath(path)
	if err != nil {
		return fmt.Errorf("caminho de ativo inválido: %w", err)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("arquivo não encontrado: '%s'", resolvedPath)
		}
		return fmt.Errorf("erro ao verificar arquivo '%s': %w", resolvedPath, err)
	}

	if info.IsDir() {
		return fmt.Errorf("o caminho apontado é um diretório, não um arquivo: '%s'", resolvedPath)
	}

	if len(allowedExtensions) > 0 {
		ext := strings.ToLower(filepath.Ext(resolvedPath))
		valid := false
		for _, allowed := range allowedExtensions {
			if ext == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("formato de arquivo '%s' não suportado. Esperado um dos: %v", ext, allowedExtensions)
		}
	}

	return nil
}

// CreateTempDir cria uma pasta temporária isolada e retorna seu caminho
func (am *AssetManager) CreateTempDir() (string, error) {
	temp, err := os.MkdirTemp("", "crom-render-*")
	if err != nil {
		return "", fmt.Errorf("falha ao criar diretório temporário: %w", err)
	}
	am.tempDir = temp
	return temp, nil
}

// Cleanup deleta a pasta temporária criada e todo o seu conteúdo
func (am *AssetManager) Cleanup() error {
	if am.tempDir == "" {
		return nil
	}
	if err := os.RemoveAll(am.tempDir); err != nil {
		return fmt.Errorf("falha ao remover diretório temporário '%s': %w", am.tempDir, err)
	}
	am.tempDir = ""
	return nil
}

// GetTempDir retorna o diretório temporário atual, se houver
func (am *AssetManager) GetTempDir() string {
	return am.tempDir
}
