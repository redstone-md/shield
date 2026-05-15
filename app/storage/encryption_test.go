package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptor_Roundtrip(t *testing.T) {
	enc, err := NewEncryptor("test-secret-key")
	require.NoError(t, err)

	plaintext := "hello, world!"
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ciphertext)
	assert.True(t, IsEncrypted(ciphertext))

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncryptor_EmptyString(t *testing.T) {
	enc, err := NewEncryptor("test-secret-key")
	require.NoError(t, err)

	result, err := EncryptField(enc, "")
	require.NoError(t, err)
	assert.Empty(t, result)

	result, err = DecryptField(enc, "")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestEncryptor_EncryptField_NonEmpty(t *testing.T) {
	enc, err := NewEncryptor("test-secret-key")
	require.NoError(t, err)

	result, err := EncryptField(enc, "sensitive data")
	require.NoError(t, err)
	assert.NotEqual(t, "sensitive data", result)
	assert.True(t, IsEncrypted(result))

	decrypted, err := DecryptField(enc, result)
	require.NoError(t, err)
	assert.Equal(t, "sensitive data", decrypted)
}

func TestEncryptor_DifferentKeysFailToDecrypt(t *testing.T) {
	enc1, err := NewEncryptor("key-one")
	require.NoError(t, err)

	enc2, err := NewEncryptor("key-two")
	require.NoError(t, err)

	ciphertext, err := enc1.Encrypt("secret message")
	require.NoError(t, err)

	_, err = enc2.Decrypt(ciphertext)
	assert.Error(t, err)
}

func TestEncryptor_IsEncrypted(t *testing.T) {
	enc, err := NewEncryptor("test-secret-key")
	require.NoError(t, err)

	encrypted, err := enc.Encrypt("test")
	require.NoError(t, err)

	assert.True(t, IsEncrypted(encrypted))
	assert.False(t, IsEncrypted("plain text"))
	assert.False(t, IsEncrypted(""))
}

func TestEncryptor_DecryptInvalidInput(t *testing.T) {
	enc, err := NewEncryptor("test-secret-key")
	require.NoError(t, err)

	_, err = enc.Decrypt("not-hex-at-all!!!")
	require.Error(t, err)

	_, err = enc.Decrypt("enc:abcd")
	assert.Error(t, err)
}

func TestEncryptor_NewEncryptor(t *testing.T) {
	enc, err := NewEncryptor("any-key-works")
	require.NoError(t, err)
	assert.NotNil(t, enc)
}

func TestEncryptor_EmptyKey(t *testing.T) {
	_, err := NewEncryptor("")
	assert.Error(t, err)
}
