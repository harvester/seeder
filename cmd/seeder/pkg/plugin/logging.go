package plugin

import "fmt"

func genHeaderMessage(message string) string {
	return fmt.Sprintf("\n🚜%s 🚜", message)
}

func genPassMessage(message string) string {
	return fmt.Sprintf("✔ %s", message)
}

func genFailMessage(message string) string {
	return fmt.Sprintf("❌ %s", message)
}

func genErrorMessage(err error) string {
	return fmt.Sprintf("⛔execution stopped: %s", err.Error())
}
