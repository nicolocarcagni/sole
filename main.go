package main

import (
	"fmt"
)

func main() {
	printWelcome()
	Execute()
}

func printWelcome() {
	// Yellow/Orange ASCII Art
	fmt.Println("\033[33m")
	fmt.Println("   _____  ____  _      ______ ")
	fmt.Println("  / ____|/ __ \\| |    |  ____|")
	fmt.Println(" | (___ | |  | | |    | |__   ")
	fmt.Println("  \\___ \\| |  | | |    |  __|  ")
	fmt.Println("  ____) | |__| | |____| |____ ")
	fmt.Println(" |_____/ \\____/|______|______|")
	fmt.Println("\033[0m")
	fmt.Println("\033[36m   Sole Blockchain v1.0 (Educational)\033[0m")
	fmt.Println("\033[90m   (c) 2026 Università del Salento\033[0m")
}
