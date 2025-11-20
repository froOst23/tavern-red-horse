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
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Type          string     `json:"type"`
	Difficult     string     `json:"difficult"`
	VictoryEffect string     `json:"victory_effect"`
	DefeatEffect  string     `json:"defeat_effect"`
	Requirement   string     `json:"requirement"`
	ImagePath     *string    `json:"image_path,omitempty"`
	Current       bool       `json:"current"`
	Used          bool       `json:"used"`
	Init          bool       `json:"init"`
	CreatedAt     *time.Time `json:"created_at"`
}
