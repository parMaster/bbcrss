# BBC RSS Feed Parser with Frontend

## Description

This is a simple RSS feed parser that fetches the latest news from the BBC website and displays it in a simple frontend.

## Running the project

1. Clone the repository
2. Run the following command in the root directory of the project

```sh
docker compose up --build
```

3. Open your browser and navigate to `http://localhost:8080`

It will run the postgres containter, apply necessary migrations, run rabbit and the app container.

## Testing

To run the tests, run the following command in the root directory of the project

```sh
go test ./...
```

Testing utilizes the testcontainers library to spin up a postgres container and run the tests against it. Docker must be installed on the machine to run the tests.
