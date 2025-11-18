package models

import "time"

type Team struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	Health    int        `json:"health"`
	Drunk     int        `json:"drunk"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type Event struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ImagePath   *string    `json:"image_path,omitempty"`
	Used        bool       `json:"used"`
	Current     bool       `json:"current"`
	CreatedAt   *time.Time `json:"created_at"`
}
