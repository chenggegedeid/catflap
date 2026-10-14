package jwt

import (
	"errors"
	"fmt"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// Subject 单用户面板，固定 subject
const Subject = "admin"

// Claims JWT声明（单用户，无角色）
type Claims struct {
	jwt.RegisteredClaims
}

// GenerateToken 用给定密钥签发一个有效期为 ttl 的 token
func GenerateToken(secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   Subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 用给定密钥解析并校验 token。
//
// 三道锁一起用，防的是 JWT 的经典坑「算法混淆」：攻击者把 header 里的 alg
// 改成 none 或 RS256，骗服务端用错的算法去验签（比如把 HMAC 密钥当 RSA 公钥使），
// 从而伪造出任意 token。
//  1. WithValidMethods 只认 HS256 —— alg: none 在这里就被挡下；
//  2. keyfunc 里再确认一次签名方法确实是 HMAC 族，双保险；
//  3. WithExpirationRequired 要求 token 必须带 exp，没有 exp 的一律无效。
func ParseToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("不支持的签名算法：%v", token.Header["alg"])
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("token无效")
	}
	return claims, nil
}
