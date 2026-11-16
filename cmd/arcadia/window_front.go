package main

import (
	"errors"
	"fmt"
)

// BringToFront puts the Arcadia window in the foreground (CON-023). It is local: it never calls the
// agent. A refusal by Windows is not an error.
func (a *App) BringToFront() error {
	if a.platform.bringToFront == nil {
		return errors.New("desktop: bringToFront indisponível")
	}
	fg, err := a.platform.bringToFront()
	if err != nil {
		a.logger().Debug("falha ao trazer a janela ao primeiro plano", "error", err.Error())
		return fmt.Errorf("desktop: bringToFront: %w", err)
	}
	a.logger().Debug("janela ao primeiro plano", "foreground", fg)
	return nil
}
