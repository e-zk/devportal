FROM golang:alpine

RUN apk --no-cache add libc-dev gcc

WORKDIR /app

COPY go.mod .
COPY go.sum .
RUN go mod download
RUN go install github.com/mattn/go-sqlite3

COPY . .

RUN go build -o /devportal

EXPOSE 8080

CMD [ "/devportal" ]
