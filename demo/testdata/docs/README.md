# todo

A tiny todo API used to demo [lazyreview](https://github.com/adelplace/lazyreview).

## Run

```sh
go run ./cmd/server          # listens on :8080
ADDR=:9000 go run ./cmd/server
```

## Endpoints

| Method | Path                  | Description         |
| ------ | --------------------- | ------------------- |
| GET    | `/todos`              | list todos          |
| POST   | `/todos`              | create a todo       |
| GET    | `/todos/{id}`         | get a todo          |
| POST   | `/todos/{id}/toggle`  | toggle done         |
| DELETE | `/todos/{id}`         | delete a todo       |

## Example

```sh
curl -X POST localhost:8080/todos -d '{"title": "Ship it"}'
curl localhost:8080/todos
```
