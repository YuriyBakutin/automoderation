package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/thanhpk/randstr"
)

// AuthRequest represents auth request format
type AuthRequest struct {
	RoomName string `json:"room"`
	UserName string `json:"name"`
}

// TokenResponse represents auth response format for registered user
type TokenResponse struct {
	Token string `json:"token"`
}

// ErrorResponse represents auth response format for dismissed user
type ErrorResponse struct {
	Error string `json:"error"`
}

func (storage *roomStorage) serveAuth(w http.ResponseWriter, r *http.Request) {
	var authRequest AuthRequest
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&authRequest)

	if err != nil || authRequest.RoomName == "" || authRequest.UserName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	room := storage.getOrCreateRoom(authRequest.RoomName)
	if room.reserved(authRequest.UserName) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Sorry, that username is already taken"})
	}

	token := randstr.Hex(32)

	room.mux.Lock()
	defer room.mux.Unlock()

	storage.users[token] = newUser(authRequest.UserName, room)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TokenResponse{Token: token})
}

func (storage *roomStorage) serveWs(w http.ResponseWriter, r *http.Request) {
	user, ok := storage.users[r.URL.Query().Get("token")]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error:", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user.connect(conn)
	user.room.sendCurrentState(user)
}

func main() {
	storage := newRoomStorage()
	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "/data/static/index.html") }).Methods("GET")
	r.HandleFunc("/auth", storage.serveAuth).Methods("POST")
	r.HandleFunc("/ws", storage.serveWs)
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("/data/static"))))

	err := http.ListenAndServe("0.0.0.0:8080", handlers.LoggingHandler(os.Stdout, r))
	if err != nil {
		fmt.Println("ListenAndServe:", err.Error())
	}
}
