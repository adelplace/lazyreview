// Package store keeps todos in memory.
package store

import (
	"errors"
	"sort"
	"time"
)

// ErrNotFound is returned when a todo does not exist.
var ErrNotFound = errors.New("todo not found")

// Todo is a single item of the list.
type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

// Store is an in-memory todo list.
type Store struct {
	todos  map[int]Todo
	nextID int
}

func New() *Store {
	return &Store{todos: map[int]Todo{}, nextID: 1}
}

// Add creates a todo and returns it.
func (s *Store) Add(title string) Todo {
	t := Todo{ID: s.nextID, Title: title, CreatedAt: time.Now()}
	s.todos[t.ID] = t
	s.nextID++
	return t
}

// Get returns the todo with the given id.
func (s *Store) Get(id int) (Todo, error) {
	t, ok := s.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

// List returns every todo, oldest first.
func (s *Store) List() []Todo {
	out := make([]Todo, 0, len(s.todos))
	for _, t := range s.todos {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Toggle flips the done state of a todo.
func (s *Store) Toggle(id int) (Todo, error) {
	t, ok := s.todos[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.Done = !t.Done
	s.todos[id] = t
	return t, nil
}

// Delete removes a todo.
func (s *Store) Delete(id int) error {
	if _, ok := s.todos[id]; !ok {
		return ErrNotFound
	}
	delete(s.todos, id)
	return nil
}
