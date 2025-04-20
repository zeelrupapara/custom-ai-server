package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/zeelrupapara/custom-ai-server/pkg/config"
	"github.com/zeelrupapara/custom-ai-server/pkg/db"
)

// Register creates a new user
func Register(c *fiber.Ctx) error {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var body req
	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}
	fmt.Println(body)
	hash, _ := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	_, err := db.PG.Exec(c.Context(),
		`INSERT INTO users(username,password,is_admin)
		 VALUES($1,$2,false)`, body.Username, string(hash))
	if err != nil {
		return fiber.ErrConflict
	}
	return c.SendStatus(fiber.StatusCreated)
}

// Login authenticates and returns JWT
func Login(c *fiber.Ctx) error {
	type req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	var body req
	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}
	var (
		id      int
		pwHash  string
		isAdmin bool
	)
	row := db.PG.QueryRow(c.Context(),
		"SELECT id,password,is_admin FROM users WHERE username=$1", body.Username)
	if err := row.Scan(&id, &pwHash, &isAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fmt.Println(err)
			return fiber.ErrUnauthorized
		}
		fmt.Println(err)
		return fiber.ErrInternalServerError
	}
	if bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(body.Password)) != nil {
		return fiber.ErrUnauthorized
	}
	// Create JWT
	claims := jwt.MapClaims{
		"sub":   id,
		"admin": isAdmin,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
	}
	fmt.Println(claims)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret := config.Load().JWTSecret
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Println(err)
		return fiber.ErrInternalServerError
	}
	return c.JSON(fiber.Map{"token": signed})
}

// Protect is middleware validating JWT; if requireAdmin, also checks "admin" claim.
func Protect(requireAdmin bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		h := c.Get("Authorization")
		if len(h) < 7 || h[:7] != "Bearer " {
			return fiber.ErrUnauthorized
		}
		tkn := h[7:]
		token, err := jwt.Parse(tkn, func(t *jwt.Token) (interface{}, error) {
			return []byte(config.Load().JWTSecret), nil
		})
		if err != nil || !token.Valid {
			return fiber.ErrUnauthorized
		}
		claims := token.Claims.(jwt.MapClaims)
		if requireAdmin && claims["admin"] != true {
			return fiber.ErrForbidden
		}
		c.Locals("userID", int(claims["sub"].(float64)))
		return c.Next()
	}
}
