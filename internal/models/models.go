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
	Type          string     `json:"type,omitempty"`
	Difficult     string     `json:"difficult,omitempty"`
	VictoryEffect string     `json:"victory_effect,omitempty"`
	DefeatEffect  string     `json:"defeat_effect,omitempty"`
	Requirement   string     `json:"requirement,omitempty"`
	ImagePath     *string    `json:"image_path,omitempty"`
	Current       bool       `json:"current"`
	Used          bool       `json:"used"`
	Init          bool       `json:"init"`
	CreatedAt     *time.Time `json:"created_at"`
}

type Player struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	TeamID    int       `json:"team_id"`
	TeamName  string    `json:"team_name,omitempty"`
	HasMoved  bool      `json:"has_moved"`
	IsCurrent bool      `json:"is_current"`
	TurnOrder int       `json:"turn_order"`
	Skip      bool      `json:"skip"`
	CreatedAt time.Time `json:"created_at"`
}

type GameState struct {
	ID           int       `json:"id"`
	CurrentRound int       `json:"current_round"`
	IsActive     bool      `json:"is_active"`
	UpdatedAt    time.Time `json:"updated_at"`
}
