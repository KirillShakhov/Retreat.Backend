package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Email        string             `bson:"email" json:"email"`
	PasswordHash string             `bson:"password_hash" json:"-"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
}

type UserStore struct {
	mongodb *MongoDB
}

func NewUserStore(mongodb *MongoDB) *UserStore {
	return &UserStore{
		mongodb: mongodb,
	}
}

func (us *UserStore) CreateUser(email, password string) error {
	key := strings.ToLower(strings.TrimSpace(email))
	if key == "" || password == "" {
		return errors.New("empty email or password")
	}

	collection := us.mongodb.GetCollection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if user already exists
	var existingUser User
	err := collection.FindOne(ctx, bson.M{"email": key}).Decode(&existingUser)
	if err == nil {
		return errors.New("user already exists")
	}
	if err != mongo.ErrNoDocuments {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := User{
		Email:        email,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}

	_, err = collection.InsertOne(ctx, user)
	return err
}

func (us *UserStore) VerifyUser(email, password string) error {
	key := strings.ToLower(strings.TrimSpace(email))

	collection := us.mongodb.GetCollection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user User
	err := collection.FindOne(ctx, bson.M{"email": key}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return errors.New("invalid credentials")
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return errors.New("invalid credentials")
	}

	return nil
}

func (us *UserStore) GetUserByEmail(email string) (*User, error) {
	key := strings.ToLower(strings.TrimSpace(email))

	collection := us.mongodb.GetCollection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user User
	err := collection.FindOne(ctx, bson.M{"email": key}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, errors.New("user not found")
		}
		return nil, err
	}

	return &user, nil
}
