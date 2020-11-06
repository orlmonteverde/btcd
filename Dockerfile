FROM golang:1.15.3-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build app
RUN CGO_ENABLED=0 go build -o /app/btcd .
RUN CGO_ENABLED=0 go build -o /app/btcctl /app/cmd/btcctl
RUN CGO_ENABLED=0 go build -o /app/addblock /app/cmd/addblock
RUN CGO_ENABLED=0 go build -o /app/findcheckpoint /app/cmd/findcheckpoint
RUN CGO_ENABLED=0 go build -o /app/gencerts /app/cmd/gencerts

FROM alpine:3.11.2
WORKDIR /bin
COPY --from=builder /app/btcd .
COPY --from=builder /app/btcctl .
COPY --from=builder /app/addblock .
COPY --from=builder /app/findcheckpoint .
COPY --from=builder /app/gencerts .

ENV PORT=18334

# RegressionNetParams Port.
EXPOSE 18334

# Default ports.
EXPOSE ${PORT}

CMD [ "btcd" ]
