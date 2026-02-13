package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

type AESCrypto struct {
	key   []byte
	block cipher.Block
	gcm   cipher.AEAD
}

func NewAESCrypto(key []byte) (*AESCrypto, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESCrypto{key: key, block: block, gcm: gcm}, nil
}

func NewAESCryptoFromHex(hexKey string) (*AESCrypto, error) {
	key, err := base64.StdEncoding.DecodeString(hexKey)
	if err != nil {
		// 尝试直接使用字符串
		key = []byte(hexKey)
	}
	if len(key) < 32 {
		// 补齐到 32 字节
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	} else if len(key) > 32 {
		key = key[:32]
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESCrypto{key: key, block: block, gcm: gcm}, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func GenerateKeyBase64() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func (c *AESCrypto) Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext = c.gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (c *AESCrypto) Decrypt(ciphertext, nonce []byte) (plaintext []byte, err error) {
	return c.gcm.Open(nil, nonce, ciphertext, nil)
}

func (c *AESCrypto) EncryptToBase64(plaintext []byte) (ciphertextB64, nonceB64 string, err error) {
	ciphertext, nonce, err := c.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext),
		base64.StdEncoding.EncodeToString(nonce), nil
}

func (c *AESCrypto) DecryptFromBase64(ciphertextB64, nonceB64 string) (plaintext []byte, err error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, err
	}
	return c.Decrypt(ciphertext, nonce)
}
