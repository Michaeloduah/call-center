package auth

import (
	"strings"

	"callcenter/internal/platform/validate"
)

// Request DTOs. Each validates and normalises itself; services receive only validated values.

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (in *LoginInput) Validate() error {
	v := validate.Errors{}
	in.Email = v.Email("email", in.Email)
	if in.Password == "" {
		v.Add("password", "This field is required.")
	}
	return v.Err()
}

type SignupInput struct {
	CompanyName string `json:"company_name"`
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (in *SignupInput) Validate() error {
	v := validate.Errors{}
	in.CompanyName = v.RequiredString("company_name", in.CompanyName, 120)
	in.FullName = v.RequiredString("full_name", in.FullName, 120)
	in.Email = v.Email("email", in.Email)
	v.Password("password", in.Password)
	return v.Err()
}

type EmailInput struct {
	Email string `json:"email"`
}

func (in *EmailInput) Validate() error {
	v := validate.Errors{}
	in.Email = v.Email("email", in.Email)
	return v.Err()
}

type TokenInput struct {
	Token string `json:"token"`
}

const maxTokenLen = 128

func (in *TokenInput) Validate() error {
	v := validate.Errors{}
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" || len(in.Token) > maxTokenLen {
		return errInvalidToken
	}
	return v.Err()
}

type TokenPasswordInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

func (in *TokenPasswordInput) Validate() error {
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" || len(in.Token) > maxTokenLen {
		return errInvalidToken
	}
	v := validate.Errors{}
	v.Password("password", in.Password)
	return v.Err()
}
