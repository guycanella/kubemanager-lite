package docker

import (
	"bufio"
	"context"
	"fmt"

	"kubemanager_lite/backend/streaming"

	"github.com/docker/docker/api/types/container"
)

// LogHub is the interface accepted by LogStreamer to forward log messages.
// Using an interface instead of *streaming.Hub allows mock injection in tests.
type LogHub interface {
	Send(msg streaming.LogMessage)
}

// LogStreamer manages active log streams per container.
type LogStreamer struct {
	client  *Client
	hub     LogHub
	streams map[string]context.CancelFunc
}

// NewLogStreamer creates a LogStreamer connected to the Hub.
func NewLogStreamer(client *Client, hub LogHub) *LogStreamer {
	return &LogStreamer{
		client:  client,
		hub:     hub,
		streams: make(map[string]context.CancelFunc),
	}
}

// StartStream inicia o streaming de logs de um container específico.
// Cada container recebe sua própria goroutine e context de cancelamento.
//
// O fluxo é:
//  1. Abre stream de logs do Docker (follow=true, timestamps=true)
//  2. Lê linha a linha via bufio.Scanner
//  3. Envia cada linha para o Hub central (que aplica o backpressure)
//
// Se o stream já estiver ativo para este container, não abre um segundo.
func (ls *LogStreamer) StartStream(containerID, containerName string) error {
	// Evita streams duplicados
	if _, exists := ls.streams[containerID]; exists {
		return nil
	}

	// Context com cancel — guardamos a cancel func para poder parar depois
	ctx, cancel := context.WithCancel(context.Background())
	ls.streams[containerID] = cancel

	go func() {
		defer func() {
			// Limpeza: remove o stream do map quando encerrar
			delete(ls.streams, containerID)
		}()

		if err := ls.streamLogs(ctx, containerID, containerName); err != nil {
			// Não logamos context.Canceled — é o comportamento esperado
			// quando o usuário para o stream manualmente.
			if ctx.Err() == nil {
				fmt.Printf("[LogStreamer] Erro no stream de %s: %v\n", containerName, err)
			}
		}
	}()

	return nil
}

// StopStream cancela o stream de logs de um container.
// O context.Cancel() faz o goroutine de leitura encerrar naturalmente.
func (ls *LogStreamer) StopStream(containerID string) {
	if cancel, exists := ls.streams[containerID]; exists {
		cancel()
	}
}

// StopAll cancela todos os streams ativos. Chamado no shutdown da aplicação.
func (ls *LogStreamer) StopAll() {
	for id, cancel := range ls.streams {
		cancel()
		delete(ls.streams, id)
	}
}

// ActiveStreams retorna os IDs dos containers com streams ativos.
func (ls *LogStreamer) ActiveStreams() []string {
	ids := make([]string, 0, len(ls.streams))
	for id := range ls.streams {
		ids = append(ids, id)
	}
	return ids
}

// streamLogs faz a leitura contínua dos logs de um container.
// Esta função roda dentro de uma goroutine e bloqueia até:
//   - O context ser cancelado (usuário fecha a aba)
//   - O container ser parado
//   - Um erro de I/O ocorrer
func (ls *LogStreamer) streamLogs(ctx context.Context, containerID, containerName string) error {
	reader, err := ls.client.cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true, // mantém a conexão aberta (tail -f)
		Timestamps: true, // inclui timestamp em cada linha
		Tail:       "50", // últimas 50 linhas ao conectar (não sobrecarrega no início)
	})
	if err != nil {
		return fmt.Errorf("erro ao abrir stream de logs: %w", err)
	}
	defer reader.Close()

	// bufio.Scanner lê linha a linha de forma eficiente.
	// O Docker multiplexes stdout/stderr num único stream com um header de 8 bytes.
	// O Scanner remove esses headers automaticamente para nós.
	scanner := bufio.NewScanner(reader)

	// Aumenta o buffer padrão para lidar com linhas de log muito longas
	const maxLineSize = 1024 * 1024 // 1MB por linha
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLineSize)

	for scanner.Scan() {
		// Verifica se o context foi cancelado antes de processar a próxima linha
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()

		// Remove o header de 8 bytes do multiplexer do Docker se presente
		// Formato: [STREAM_TYPE(1)] [0 0 0(3)] [SIZE(4)] [PAYLOAD]
		if len(line) > 8 {
			line = line[8:]
		}

		ls.hub.Send(streaming.LogMessage{
			Source: "docker",
			ID:     containerID,
			Name:   containerName,
			Line:   line,
		})
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil // cancelamento esperado
		}
		return fmt.Errorf("erro na leitura do stream: %w", err)
	}

	return nil
}
