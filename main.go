package main

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	mcrcon "github.com/Kelwing/mc-rcon"
)

var worlds []string
var usernameRe *regexp.Regexp

func init() {
	const reg = `^\w+$`
	usernameRe = regexp.MustCompile(reg)

}

func main() {
	go func() {
		for {
			newWorlds := loadWorlds()
			if len(newWorlds) != 0 {
				worlds = newWorlds
			}
			time.Sleep(1 * time.Minute)
		}
	}()

	srv := server{}
	srv.tmpl = ReloadingTmpl{"templates/*"}

	http.HandleFunc("GET /{$}", srv.landingPage)
	http.HandleFunc("GET /signup", srv.signupPage)
	http.HandleFunc("POST /whitelist", srv.whitelistUser)

	http.Handle("/static/", http.FileServer(http.Dir(".")))

	addr := "0.0.0.0:8080"
	slog.Info("Server starting", "addr", addr)
	http.ListenAndServe(addr, nil)
}

type server struct {
	tmpl Templater
}

func (s server) landingPage(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "landing.html.tmpl", nil)
}

func (s server) signupPage(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "signup.html.tmpl", nil)
}

func (s server) whitelistUser(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")

	if err := validUser(username); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%v", err)
		return
	}

	err := whitelist(username)
	if err == nil {
		fmt.Fprintf(w, "User %s has been whitelisted!", username)
	} else if err == errNoUser {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%v", err)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "there was an issue (devoxel might be an idiot): %v", err)
	}

	return
}

type worldsView struct {
	Worlds []string
}

func (s server) worldsPage(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "worlds.html.tmpl", worldsView{Worlds: worlds})
}

func loadWorlds() []string {
	files, err := os.ReadDir("./worlds")
	if err != nil {
		slog.Error("error reading worlds", "error", err)
		return []string{}
	}

	for _, file := range files {
		if file.IsDir() {
			worlds = append(worlds, file.Name())
		}
	}
	return worlds
}

type Templater interface {
	ExecuteTemplate(wr io.Writer, name string, data interface{}) error
}

type ReloadingTmpl struct {
	Glob string
}

func (e ReloadingTmpl) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	t := template.Must(template.ParseGlob(e.Glob))
	return t.ExecuteTemplate(wr, name, data)
}

func validUser(username string) error {
	if len(username) < 3 || len(username) > 16 || !usernameRe.MatchString(username) {
		return errors.New("username is invalid. RULES: 3-16 chars, matches re =" + usernameRe.String())
	}
	return nil
}

var errNoUser = errors.New("that player does not exist")

func whitelist(username string) error {
	passwd := "bigoldpileofbricks"

	// Need to validate username is reasonable.
	// I dont want to see somebody trying to exploit my server via some weird ass backdoor in the RCON protocol.
	// Serializator on spigotmc.org forum dug up these validation rules, going to do the same shit here.

	conn := new(mcrcon.MCConn)
	err := conn.Open("localhost:25575", passwd)
	if err != nil {
		log.Fatalln("Open failed", err)
	}
	defer conn.Close()

	err = conn.Authenticate()
	if err != nil {
		log.Fatalln("Auth failed", err)
	}

	resp, err := conn.SendCommand(fmt.Sprintf("whitelist add %s", username))
	if err != nil {
		log.Fatalln("Command failed", err)
	}
	resp = strings.ToLower(resp)

	if resp == "player is already whitelisted" {
		return nil
	}
	goodResp := fmt.Sprintf("added %s to the whitelist", strings.ToLower(username))
	if resp == goodResp {
		return nil
	}

	if resp == "that player does not exist" {
		return errNoUser
	}

	return errors.New(fmt.Sprintf("error: output doesnt look right: %v", resp))
}
