module example.com/fixture

go 1.22

require example.com/m v1.2.3

require (
	example.com/direct v0.5.0
	example.com/indirect v0.9.0 // indirect
)
