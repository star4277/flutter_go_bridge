package api

import "C"
import "fmt"

// fgb(sync)
func Greeting(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

type Point struct {
	X     int
	Y     int
	Label string
}

func (p *Point) Move(x int, y int) {
	p.X = x
	p.Y = y
}
