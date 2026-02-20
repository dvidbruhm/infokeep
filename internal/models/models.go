package models

type TagCount struct {
	Tag   string
	Count int
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    string
}
