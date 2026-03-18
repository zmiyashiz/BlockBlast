package main

import (
	"fmt"
	"net/http"
)

func main() {
	fmt.Println("Сервер запущен: http://localhost:8080")
	http.ListenAndServe(":8080", http.FileServer(http.Dir(".")))
}
