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

## Notes
- The project is structured in a flat manner, as there are not many files and it is a test task that is convenient to view in such a flat structure.
- I used a simple template renderer for the frontend, so I did not create REST endpoints, as the task does not require them, but the structure for this is ready.
- I used a ready-made package for parsing rss, as the task does not require manual parsing.

## Notes on "What I learned"
- Pretty straightforward task, 80+% I had ideas how to implement right away. Some caviats with docker-compose along the way - especially with the migrate tool (GOOS=linus did the trick)
- Using testcontainers to run tests against multiple containers - pretty cool and useful, but my 10 year old laptop is not happy about it
- Applied testserver pattern for testing the http server, which I learned recently
- First time running tests this huge in github actions, surprisingly easy to set up