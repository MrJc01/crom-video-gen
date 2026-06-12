package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetManager_ValidateFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "asset_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	am := NewAssetManager(tempDir)

	// Cria arquivo de teste válido (.jpg)
	testFile := filepath.Join(tempDir, "imagem.jpg")
	if err := os.WriteFile(testFile, []byte("fake image data"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Validar arquivo existente com extensão permitida
	if err := am.ValidateFile("imagem.jpg", []string{".jpg", ".png"}); err != nil {
		t.Errorf("Esperava arquivo válido, obteve erro: %v", err)
	}

	// 2. Validar arquivo existente com extensão não permitida
	if err := am.ValidateFile("imagem.jpg", []string{".png", ".gif"}); err == nil {
		t.Error("Esperava erro por extensão não permitida, mas passou")
	}

	// 3. Validar arquivo inexistente
	if err := am.ValidateFile("inexistente.jpg", nil); err == nil {
		t.Error("Esperava erro de arquivo inexistente, mas passou")
	}

	// 4. Validar diretório (não deve ser aceito como arquivo regular)
	subDir := filepath.Join(tempDir, "pasta")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := am.ValidateFile("pasta", nil); err == nil {
		t.Error("Esperava erro porque o caminho é um diretório, mas passou")
	}
}

func TestAssetManager_CreateTempDirAndCleanup(t *testing.T) {
	am := NewAssetManager(".")

	tempDir, err := am.CreateTempDir()
	if err != nil {
		t.Fatalf("Erro ao criar temp dir: %v", err)
	}

	// Verifica se a pasta temporária foi criada no disco
	info, err := os.Stat(tempDir)
	if err != nil {
		t.Fatalf("Temp dir não existe no disco: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("Temp dir não é um diretório")
	}

	// Cria arquivo dentro da pasta temporária
	tempFile := filepath.Join(tempDir, "temp.txt")
	if err := os.WriteFile(tempFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Cleanup
	if err := am.Cleanup(); err != nil {
		t.Fatalf("Erro no cleanup: %v", err)
	}

	// Garante que a pasta foi deletada
	_, err = os.Stat(tempDir)
	if err == nil {
		t.Error("Temp dir ainda existe no disco após o cleanup")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Esperava erro de inexistência, obteve: %v", err)
	}
}
