package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key))
	}

	// 生成的密钥应该是唯一的
	key2, _ := GenerateKey()
	if bytes.Equal(key, key2) {
		t.Error("Generated keys should be unique")
	}
}

func TestNewAESCrypto(t *testing.T) {
	// 测试有效密钥
	key, _ := GenerateKey()
	crypto, err := NewAESCrypto(key)
	if err != nil {
		t.Fatalf("NewAESCrypto() error: %v", err)
	}
	if crypto == nil {
		t.Fatal("Expected crypto instance, got nil")
	}

	// 测试无效密钥长度
	shortKey := make([]byte, 16)
	_, err = NewAESCrypto(shortKey)
	if err == nil {
		t.Error("Expected error for short key")
	}
}

func TestNewAESCryptoFromHex(t *testing.T) {
	// 生成有效的 base64 密钥
	key, _ := GenerateKey()
	b64Key := hex.EncodeToString(key) // 实际上函数会尝试 base64 解码，失败后直接用字符串

	crypto, err := NewAESCryptoFromHex(b64Key)
	if err != nil {
		t.Fatalf("NewAESCryptoFromHex() error: %v", err)
	}
	if crypto == nil {
		t.Fatal("Expected crypto instance, got nil")
	}

	// 函数设计为宽松模式，不会返回错误
	// 短密钥会被补齐到 32 字节
	crypto2, err := NewAESCryptoFromHex("short")
	if err != nil {
		t.Errorf("NewAESCryptoFromHex should not error for short key: %v", err)
	}
	if crypto2 == nil {
		t.Error("Expected crypto instance for short key")
	}

	// 测试加解密是否能正常工作
	plaintext := []byte("test message")
	ciphertext, nonce, err := crypto2.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}
	decrypted, err := crypto2.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("Decrypted text doesn't match original")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key, _ := GenerateKey()
	crypto, _ := NewAESCrypto(key)

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hello")},
		{"medium", []byte("this is a medium length message for testing")},
		{"long", bytes.Repeat([]byte("a"), 10000)},
		{"binary", []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, nonce, err := crypto.Encrypt(tc.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error: %v", err)
			}

			// Nonce 应该是 12 字节 (GCM 标准)
			if len(nonce) != 12 {
				t.Errorf("Expected nonce length 12, got %d", len(nonce))
			}

			// 解密
			decrypted, err := crypto.Decrypt(ciphertext, nonce)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}

			if !bytes.Equal(decrypted, tc.plaintext) {
				t.Errorf("Decrypted data doesn't match original")
			}
		})
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, _ := GenerateKey()
	key2, _ := GenerateKey()

	crypto1, _ := NewAESCrypto(key1)
	crypto2, _ := NewAESCrypto(key2)

	plaintext := []byte("secret message")
	ciphertext, nonce, _ := crypto1.Encrypt(plaintext)

	// 使用错误的密钥解密应该失败
	_, err := crypto2.Decrypt(ciphertext, nonce)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key")
	}
}

func TestDecryptWithWrongNonce(t *testing.T) {
	key, _ := GenerateKey()
	crypto, _ := NewAESCrypto(key)

	plaintext := []byte("secret message")
	ciphertext, _, _ := crypto.Encrypt(plaintext)

	// 使用错误的 nonce 解密应该失败
	wrongNonce := make([]byte, 12)
	_, err := crypto.Decrypt(ciphertext, wrongNonce)
	if err == nil {
		t.Error("Expected error when decrypting with wrong nonce")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key, _ := GenerateKey()
	crypto, _ := NewAESCrypto(key)

	plaintext := []byte("same message")

	ciphertext1, nonce1, _ := crypto.Encrypt(plaintext)
	ciphertext2, nonce2, _ := crypto.Encrypt(plaintext)

	// 即使明文相同，密文也应该不同 (因为 nonce 不同)
	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Error("Same plaintext should produce different ciphertexts")
	}

	if bytes.Equal(nonce1, nonce2) {
		t.Error("Nonces should be different")
	}
}

func BenchmarkEncrypt(b *testing.B) {
	key, _ := GenerateKey()
	crypto, _ := NewAESCrypto(key)
	plaintext := bytes.Repeat([]byte("a"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		crypto.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	key, _ := GenerateKey()
	crypto, _ := NewAESCrypto(key)
	plaintext := bytes.Repeat([]byte("a"), 1024)
	ciphertext, nonce, _ := crypto.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		crypto.Decrypt(ciphertext, nonce)
	}
}
