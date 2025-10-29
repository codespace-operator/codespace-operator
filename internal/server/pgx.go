package server

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type pgxConn struct{ *pgx.Conn }

func (p *pgxConn) Exec(ctx context.Context, sql string, args ...any) error {
    _, err := p.Conn.Exec(ctx, sql, args...)
    return err
}
func (p *pgxConn) QueryRow(ctx context.Context, sql string, args ...any) pgRow {
    return p.Conn.QueryRow(ctx, sql, args...)
}