package main

import (
	"github.com/fatih/color"
)

// UI Helpers for standardized logging

func PrintSuccess(format string, a ...interface{}) {
	color.Green("✅ "+format, a...)
}

func PrintError(format string, a ...interface{}) {
	color.Red("⛔ "+format, a...)
}

func PrintInfo(format string, a ...interface{}) {
	color.Cyan("ℹ️  "+format, a...)
}

func PrintWarning(format string, a ...interface{}) {
	color.Yellow("⚠️  "+format, a...)
}

func PrintMiner(format string, a ...interface{}) {
	// Gold/Yellow for Miner
	c := color.New(color.FgYellow, color.Bold)
	c.Printf("⛏️  "+format+"\n", a...)
}

func PrintNetwork(format string, a ...interface{}) {
	// Blue for Network
	c := color.New(color.FgBlue)
	c.Printf("🌐 "+format+"\n", a...)
}
