package main

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"golang.org/x/build/maintner"
	"golang.org/x/build/maintner/godata"
)

func main() {
	s := newServer()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, s))
}

type server struct {
	sync.Mutex
	corpus *maintner.Corpus
	t      *template.Template
}

func newServer() *server {
	s := &server{t: template.Must(template.ParseFiles("index.html"))}
	go func() {
		corpus, err := godata.Get(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		s.Lock()
		s.corpus = corpus
		s.Unlock()
	}()
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := r.FormValue("user")
	if user == "" {
		if err := s.t.Execute(w, nil); err != nil {
			log.Print(err)
		}
		return
	}

	start := time.Now()
	comments, err := s.getCommentsForUser(user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	duration := time.Since(start)

	limit, _ := strconv.Atoi(r.FormValue("limit"))
	if limit == 0 {
		limit = 100
	}
	if limit > len(comments) {
		limit = len(comments)
	}

	if err := s.t.Execute(w, struct {
		Comments   []comment
		TotalCount int
		Duration   time.Duration
	}{Comments: comments[:limit], TotalCount: len(comments), Duration: duration}); err != nil {
		log.Print(err)
	}
}

type comment struct {
	GR *maintner.GitHubRepo
	GI *maintner.GitHubIssue
	GC *maintner.GitHubComment
}

func (s *server) getCommentsForUser(user string) ([]comment, error) {
	s.Lock()
	defer s.Unlock()
	if s.corpus == nil {
		return nil, errors.New("not ready")
	}

	var comments []comment
	if err := s.corpus.GitHub().ForeachRepo(func(gr *maintner.GitHubRepo) error {
		return gr.ForeachIssue(func(gi *maintner.GitHubIssue) error {
			return gi.ForeachComment(func(gc *maintner.GitHubComment) error {
				if user == gc.User.Login {
					comments = append(comments, comment{GR: gr, GI: gi, GC: gc})
				}
				return nil
			})
		})
	}); err != nil {
		return nil, err
	}

	sort.Slice(comments, func(i, j int) bool { return comments[i].GC.Created.After(comments[j].GC.Created) })
	return comments, nil
}
