package relay

import (
	"bytes"
	"testing"
)

func TestSharedCryptoInit(t *testing.T) {
	// 验证共享加密器已正确初始化
	if sharedCrypto == nil {
		t.Fatal("sharedCrypto should be initialized")
	}
}

func TestSharedKeyEncryptDecrypt(t *testing.T) {
	testCases := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "small_payload",
			payload: []byte("hello world"),
		},
		{
			name:    "medium_payload",
			payload: bytes.Repeat([]byte("test"), 256), // 1KB
		},
		{
			name:    "large_payload",
			payload: bytes.Repeat([]byte("data"), 8192), // 32KB
		},
		{
			name:    "empty_payload",
			payload: []byte{},
		},
		{
			name:    "single_byte",
			payload: []byte{0x42},
		},
		{
			name:    "binary_data",
			payload: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0xfd},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.payload) == 0 {
				// 空 payload 不加密
				return
			}

			// 加密
			ciphertext, nonce, err := sharedCrypto.Encrypt(tc.payload)
			if err != nil {
				t.Fatalf("Encrypt() error: %v", err)
			}

			if len(nonce) != nonceSize {
				t.Errorf("Nonce size mismatch: got %d, want %d", len(nonce), nonceSize)
			}

			// 密文应该比明文长（包含认证标签）
			if len(ciphertext) <= len(tc.payload) {
				t.Error("Ciphertext should be longer than plaintext")
			}

			// 解密
			decrypted, err := sharedCrypto.Decrypt(ciphertext, nonce)
			if err != nil {
				t.Fatalf("Decrypt() error: %v", err)
			}

			if !bytes.Equal(decrypted, tc.payload) {
				t.Errorf("Decrypted data mismatch: got %v, want %v", decrypted, tc.payload)
			}
		})
	}
}

func TestEncryptDecryptDifferentClients(t *testing.T) {
	// 模拟两个不同的客户端使用共享密钥
	// Client A 加密，Client B 解密

	originalData := []byte("relay data from client A to client B")

	// Client A 加密
	ciphertext, nonce, err := sharedCrypto.Encrypt(originalData)
	if err != nil {
		t.Fatalf("Client A encrypt error: %v", err)
	}

	// 模拟网络传输：组装加密后的 payload
	encryptedPayload := make([]byte, nonceSize+len(ciphertext))
	copy(encryptedPayload[:nonceSize], nonce)
	copy(encryptedPayload[nonceSize:], ciphertext)

	// Client B 解密
	if len(encryptedPayload) <= nonceSize {
		t.Fatal("Encrypted payload too short")
	}

	receivedNonce := encryptedPayload[:nonceSize]
	receivedCiphertext := encryptedPayload[nonceSize:]

	decrypted, err := sharedCrypto.Decrypt(receivedCiphertext, receivedNonce)
	if err != nil {
		t.Fatalf("Client B decrypt error: %v", err)
	}

	if !bytes.Equal(decrypted, originalData) {
		t.Errorf("Data mismatch after relay: got %s, want %s", decrypted, originalData)
	}
}

func TestEncryptDecryptIntegrity(t *testing.T) {
	// 测试数据完整性：篡改密文应该导致解密失败
	data := []byte("sensitive data")

	ciphertext, nonce, err := sharedCrypto.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// 篡改密文
	if len(ciphertext) > 0 {
		ciphertext[0] ^= 0xff
	}

	// 解密应该失败
	_, err = sharedCrypto.Decrypt(ciphertext, nonce)
	if err == nil {
		t.Error("Decrypt() should fail with tampered ciphertext")
	}
}

func TestEncryptDecryptWrongNonce(t *testing.T) {
	// 测试使用错误的 nonce 应该导致解密失败
	data := []byte("test data")

	ciphertext, _, err := sharedCrypto.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// 使用错误的 nonce
	wrongNonce := make([]byte, nonceSize)
	for i := range wrongNonce {
		wrongNonce[i] = 0xff
	}

	_, err = sharedCrypto.Decrypt(ciphertext, wrongNonce)
	if err == nil {
		t.Error("Decrypt() should fail with wrong nonce")
	}
}

func TestMsgTypeDataEncryption(t *testing.T) {
	// 测试完整的消息加密流程（模拟 Send 函数的行为）
	originalPayload := []byte("message payload data")

	// 模拟 Send 中的加密逻辑
	ciphertext, nonce, err := sharedCrypto.Encrypt(originalPayload)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	encryptedPayload := make([]byte, nonceSize+len(ciphertext))
	copy(encryptedPayload[:nonceSize], nonce)
	copy(encryptedPayload[nonceSize:], ciphertext)

	// 创建消息
	msg := &TunnelMessage{
		Type:     MsgTypeData,
		StreamID: 12345,
		Payload:  encryptedPayload,
	}

	// 序列化
	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// 反序列化
	received, err := UnmarshalTunnelMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalTunnelMessage() error: %v", err)
	}

	// 模拟 readPump 中的解密逻辑
	if len(received.Payload) > nonceSize && received.Type == MsgTypeData {
		recvNonce := received.Payload[:nonceSize]
		recvCiphertext := received.Payload[nonceSize:]
		decrypted, err := sharedCrypto.Decrypt(recvCiphertext, recvNonce)
		if err != nil {
			t.Fatalf("Decrypt() error: %v", err)
		}

		if !bytes.Equal(decrypted, originalPayload) {
			t.Errorf("Payload mismatch: got %s, want %s", decrypted, originalPayload)
		}
	} else {
		t.Error("Invalid received message")
	}
}

func TestRelaySharedKeyLength(t *testing.T) {
	// 验证共享密钥长度是 32 字节（AES-256）
	if len(relaySharedKey) != 32 {
		t.Errorf("Shared key length should be 32 bytes, got %d", len(relaySharedKey))
	}
}

// ===== Benchmarks =====

func BenchmarkSharedKeyEncrypt(b *testing.B) {
	data := make([]byte, 1024) // 1KB payload
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sharedCrypto.Encrypt(data)
	}
}

func BenchmarkSharedKeyDecrypt(b *testing.B) {
	data := make([]byte, 1024)
	ciphertext, nonce, _ := sharedCrypto.Encrypt(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sharedCrypto.Decrypt(ciphertext, nonce)
	}
}

func BenchmarkSharedKeyEncryptDecrypt(b *testing.B) {
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ciphertext, nonce, _ := sharedCrypto.Encrypt(data)
		sharedCrypto.Decrypt(ciphertext, nonce)
	}
}

func BenchmarkSharedKeyEncryptLargePayload(b *testing.B) {
	data := make([]byte, 32*1024) // 32KB payload

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sharedCrypto.Encrypt(data)
	}
}

func BenchmarkFullMessageEncryptDecrypt(b *testing.B) {
	payload := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 加密
		ciphertext, nonce, _ := sharedCrypto.Encrypt(payload)
		encryptedPayload := make([]byte, nonceSize+len(ciphertext))
		copy(encryptedPayload[:nonceSize], nonce)
		copy(encryptedPayload[nonceSize:], ciphertext)

		// 创建消息并序列化
		msg := &TunnelMessage{
			Type:     MsgTypeData,
			StreamID: 12345,
			Payload:  encryptedPayload,
		}
		data, _ := msg.Marshal()

		// 反序列化并解密
		received, _ := UnmarshalTunnelMessage(data)
		if len(received.Payload) > nonceSize {
			recvNonce := received.Payload[:nonceSize]
			recvCiphertext := received.Payload[nonceSize:]
			sharedCrypto.Decrypt(recvCiphertext, recvNonce)
		}
	}
}
