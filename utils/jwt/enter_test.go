package jwt

import (
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestGenerateAndParse(t *testing.T) {
	tok, err := GenerateToken(testSecret, time.Hour)
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	claims, err := ParseToken(tok, testSecret)
	if err != nil {
		t.Fatalf("正常 token 应通过：%v", err)
	}
	if claims.Subject != Subject {
		t.Fatalf("subject = %q，期望 %q", claims.Subject, Subject)
	}
}

func TestParseTokenRejectsWrongSecret(t *testing.T) {
	tok, err := GenerateToken(testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(tok, strings.Repeat("x", 32)); err == nil {
		t.Fatal("换了密钥后仍验签通过，等于谁都能伪造 token")
	}
}

func TestParseTokenRejectsExpired(t *testing.T) {
	tok, err := GenerateToken(testSecret, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(tok, testSecret); err == nil {
		t.Fatal("已过期的 token 必须被拒绝")
	}
}

// 算法混淆：把 header 的 alg 改成 none，试图让服务端跳过验签。
func TestParseTokenRejectsAlgNone(t *testing.T) {
	claims := Claims{RegisteredClaims: jwtlib.RegisteredClaims{
		Subject:   Subject,
		IssuedAt:  jwtlib.NewNumericDate(time.Now()),
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodNone, claims)
	signed, err := tok.SignedString(jwtlib.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed, testSecret); err == nil {
		t.Fatal("alg:none 的 token 必须被拒绝")
	}
}

// 算法混淆：改成非 HMAC 家族，看 keyfunc 会不会把对称密钥交出去。
func TestParseTokenRejectsNonHMAC(t *testing.T) {
	claims := Claims{RegisteredClaims: jwtlib.RegisteredClaims{
		Subject:   Subject,
		ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	for _, m := range []jwtlib.SigningMethod{jwtlib.SigningMethodHS384, jwtlib.SigningMethodHS512} {
		tok := jwtlib.NewWithClaims(m, claims)
		signed, err := tok.SignedString([]byte(testSecret))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseToken(signed, testSecret); err == nil {
			t.Fatalf("%v 不在允许列表里，必须被拒绝", m.Alg())
		}
	}
}

// 没有 exp 的 token 等于永久有效，必须拒绝
func TestParseTokenRejectsMissingExp(t *testing.T) {
	claims := Claims{RegisteredClaims: jwtlib.RegisteredClaims{Subject: Subject}}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed, testSecret); err == nil {
		t.Fatal("没有 exp 的 token 必须被拒绝")
	}
}
