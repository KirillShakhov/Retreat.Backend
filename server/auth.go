package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Email        string    `json:"email"`
	PasswordHash string    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserStore struct {
	file  string
	users map[string]User // key: lowercase email
}

func NewUserStore(file string) (*UserStore, error) {
	us := &UserStore{
		file:  file,
		users: make(map[string]User),
	}
	if err := os.MkdirAll(filepath.Dir(file), os.ModePerm); err != nil {
		return nil, err
	}
	// Load if exists
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return us, us.save()
		}
		return nil, err
	}
	var list []User
	if len(data) != 0 {
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, err
		}
		for _, u := range list {
			us.users[strings.ToLower(u.Email)] = u
		}
	}
	return us, nil
}

func (us *UserStore) save() error {
	list := make([]User, 0, len(us.users))
	for _, u := range us.users {
		list = append(list, u)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(us.file, data, 0644)
}

func (us *UserStore) CreateUser(email, password string) error {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" || password == "" {
		return errors.New("empty email or password")
	}
	if _, exists := us.users[key]; exists {
		return errors.New("user already exists")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	us.users[key] = User{
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}
	return us.save()
}

func (us *UserStore) VerifyUser(email, password string) error {
	key := strings.ToLower(strings.TrimSpace(email))
	u, ok := us.users[key]
	if !ok {
		return errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return errors.New("invalid credentials")
	}
	return nil
}

// JWT utilities
func (server *Server) generateJWT(email string) (string, error) {
	ttl := time.Duration(server.config.TokenTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   email,
		"iat":   now.Unix(),
		"exp":   now.Add(ttl).Unix(),
		"scope": "user",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(server.config.JWTSecret))
}

func (server *Server) parseJWT(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(server.config.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("missing subject")
	}
	return sub, nil
}

// Middlewares
func (server *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

type ctxKey string

const userEmailKey ctxKey = "userEmail"

func (server *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			server.respond(w, Response{Message: "Unauthorized"}, http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(authz[len("Bearer "):])
		email, err := server.parseJWT(token)
		if err != nil {
			server.respond(w, Response{Message: "Unauthorized"}, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userEmailKey, email)
		next(w, r.WithContext(ctx))
	}
}

// Handlers
type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func (server *Server) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.respond(w, Response{Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		server.respond(w, Response{Message: "Invalid JSON"}, http.StatusBadRequest)
		return
	}
	c.Email = strings.TrimSpace(c.Email)
	if !emailRegex.MatchString(c.Email) || len(c.Password) < 6 {
		server.respond(w, Response{Message: "Invalid email or password too short"}, http.StatusBadRequest)
		return
	}
	if err := server.userStore.CreateUser(c.Email, c.Password); err != nil {
		server.respond(w, Response{Message: err.Error()}, http.StatusBadRequest)
		return
	}
	server.respond(w, Response{Message: "Registered"}, http.StatusCreated)
}

func (server *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.respond(w, Response{Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	var c credentials
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		server.respond(w, Response{Message: "Invalid JSON"}, http.StatusBadRequest)
		return
	}
	if err := server.userStore.VerifyUser(c.Email, c.Password); err != nil {
		server.respond(w, Response{Message: "Invalid credentials"}, http.StatusUnauthorized)
		return
	}
	tok, err := server.generateJWT(c.Email)
	if err != nil {
		server.respond(w, Response{Message: "Failed to issue token"}, http.StatusInternalServerError)
		return
	}
	server.respond(w, Response{Message: "Logged in", Token: tok}, http.StatusOK)
}

func (server *Server) me(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(userEmailKey).(string)
	if email == "" {
		server.respond(w, Response{Message: "Unauthorized"}, http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"email": email})
}
