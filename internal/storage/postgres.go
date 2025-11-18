package storage

import (
	"context"
	"fmt"
	"red-horse-tavern/internal/utils"
	"time"

	"red-horse-tavern/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgresFromConfig(cfg *utils.AppConfig) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.Dbname,
		cfg.Postgres.SSLMode,
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	return pool, nil
}

func (p *Postgres) GetTeams() ([]models.Team, error) {
	rows, err := p.db.Query(context.Background(), `SELECT id, name, health, drunk, updated_at FROM teams`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.Name, &t.Health, &t.Drunk, &t.UpdatedAt); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

func (p *Postgres) UpdateTeamField(id int, field string, delta int) error {
	if field != "health" && field != "drunk" {
		return fmt.Errorf("invalid field: %s", field)
	}

	_, err := p.db.Exec(context.Background(),
		fmt.Sprintf(`UPDATE teams SET %s = %s + $1, updated_at = $2 WHERE id = $3`, field, field),
		delta, time.Now(), id,
	)
	return err
}

func (p *Postgres) UpdateTeamName(id int, name string) error {
	_, err := p.db.Exec(context.Background(),
		`UPDATE teams SET name = $1, updated_at = $2 WHERE id = $3`, name, time.Now(), id)
	return err
}

func (p *Postgres) GetAllEvents() ([]models.Event, error) {
	rows, err := p.db.Query(context.Background(),
		`SELECT id, title, description, image_path, used, created_at FROM events ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		if err := rows.Scan(&e.ID, &e.Title, &e.Description, &e.ImagePath, &e.Used, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func (p *Postgres) CreateEvent(title, description string, imagePath *string) (int, error) {
	var id int
	err := p.db.QueryRow(context.Background(),
		`INSERT INTO events (title, description, image_path) VALUES ($1, $2, $3) RETURNING id`,
		title, description, imagePath).Scan(&id)
	return id, err
}

func (p *Postgres) MarkEventUsed(id int) error {
	_, err := p.db.Exec(context.Background(),
		`UPDATE events SET used = true WHERE id = $1`, id)
	return err
}

func (p *Postgres) NextEvent() (*models.Event, error) {
	var e models.Event
	err := p.db.QueryRow(context.Background(),
		`SELECT id, title, description, image_path, used, created_at 
		 FROM events WHERE used = false ORDER BY id LIMIT 1`).Scan(
		&e.ID, &e.Title, &e.Description, &e.ImagePath, &e.Used, &e.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := p.MarkEventUsed(e.ID); err != nil {
		return nil, err
	}

	return &e, nil
}
