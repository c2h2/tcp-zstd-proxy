package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/klauspost/compress/zstd"
)

var bufsize = 1024 * 1024 * 2
var compressionLevel = zstd.SpeedFastest

// flushable is an interface for writers that can flush buffered data.
type flushable interface {
	Flush() error
}

// copyWithFlush reads from src and writes to dst in chunks. After writing each
// chunk, it flushes dst if possible. It also writes the raw data to debugBuf.
func copyWithFlush(dst io.Writer, src io.Reader) error {
	buf := make([]byte, bufsize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			// Write to our debug buffer.
			/*if _, err := debugBuf.Write(buf[:n]); err != nil {
				return err
			}*/
			// Write the chunk to the destination.
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			// If dst supports flushing (e.g. a zstd encoder), flush it.
			if f, ok := dst.(flushable); ok {
				if err := f.Flush(); err != nil {
					return err
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}
	// Final flush.
	if f, ok := dst.(flushable); ok {
		return f.Flush()
	}

	return nil
}

// handleConnection accepts a client connection, connects to the target server,
// and then starts two goroutines to copy data in both directions. Depending on
// the flags, data is compressed/decompressed with zstd.
// When one side closes, both connections are closed.
func handleConnection(localConn net.Conn, targetAddr string, listenCompress, remoteCompress bool) {
	log.Println("New connection from", localConn.RemoteAddr())

	// Declare remoteConn here so that it's visible in our closure.
	var remoteConn net.Conn
	var err error
	remoteConn, err = net.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", targetAddr, err)
		localConn.Close()
		return
	}

	// Create a common cleanup function that will close both connections.
	var closeOnce sync.Once
	closeBoth := func() {
		log.Println("Closing both local and remote connections")
		localConn.Close()
		if remoteConn != nil {
			remoteConn.Close()
		}
	}
	defer closeOnce.Do(closeBoth)

	// Default: use the raw connection.
	var localReader io.Reader = localConn
	var localWriter io.Writer = localConn
	var remoteReader io.Reader = remoteConn
	var remoteWriter io.Writer = remoteConn

	// References to the zstd encoders so we can close them explicitly.
	var localZstdEncoder *zstd.Encoder
	var remoteZstdEncoder *zstd.Encoder

	// If compression is enabled on the client side.
	if listenCompress {
		log.Println("listenCompress enabled for client connection")
		localDecoder, err := zstd.NewReader(localConn)
		if err != nil {
			log.Printf("Error creating zstd decoder for client: %v", err)
			return
		}
		defer localDecoder.Close()

		localZstdEncoder, err = zstd.NewWriter(localConn, zstd.WithEncoderLevel(compressionLevel))
		if err != nil {
			log.Printf("Error creating zstd encoder for client: %v", err)
			return
		}
		// Use the wrapped reader/writer.
		localReader = localDecoder
		localWriter = localZstdEncoder
	}

	// If compression is enabled on the server side.
	if remoteCompress {
		log.Println("remoteCompress enabled for server connection")
		remoteDecoder, err := zstd.NewReader(remoteConn)
		if err != nil {
			log.Printf("Error creating zstd decoder for server: %v", err)
			return
		}
		defer remoteDecoder.Close()

		remoteZstdEncoder, err = zstd.NewWriter(remoteConn, zstd.WithEncoderLevel(compressionLevel))
		if err != nil {
			log.Printf("Error creating zstd encoder for server: %v", err)
			return
		}
		remoteReader = remoteDecoder
		remoteWriter = remoteZstdEncoder
	}

	var wg sync.WaitGroup
	wg.Add(2)
	//var bufLocalToRemote, bufRemoteToLocal bytes.Buffer

	// Client -> Server: read from localReader, write to remoteWriter.
	go func() {
		log.Println("Copying data from client to server")
		defer wg.Done()
		if err := copyWithFlush(remoteWriter, localReader); err != nil {
			log.Printf("Error copying from client to server: %v", err)
		}
		// If using compression on the remote side, close the encoder to send the stop symbol.
		if remoteCompress && remoteZstdEncoder != nil {
			if err := remoteZstdEncoder.Close(); err != nil {
				log.Printf("Error closing remote zstd encoder: %v", err)
			}
		}
		// Close both connections when this direction finishes.
		closeOnce.Do(closeBoth)
		// Uncomment for debug:
		// log.Printf("Debug (client -> server): %s", bufLocalToRemote.String())
	}()

	// Server -> Client: read from remoteReader, write to localWriter.
	go func() {
		log.Println("Copying data from server to client")
		defer wg.Done()
		if err := copyWithFlush(localWriter, remoteReader); err != nil {
			log.Printf("Error copying from server to client: %v", err)
		}
		// If using compression on the listen side, close the encoder to send the stop symbol.
		if listenCompress && localZstdEncoder != nil {
			if err := localZstdEncoder.Close(); err != nil {
				log.Printf("Error closing local zstd encoder: %v", err)
			}
		}
		// Close both connections when this direction finishes.
		closeOnce.Do(closeBoth)
		// Uncomment for debug:
		// log.Printf("Debug (server -> client): %s", bufRemoteToLocal.String())
	}()

	wg.Wait()
}

func main() {
	// Command-line flags.
	listenAddr := flag.String("listen", ":8080", "Listen address (e.g. :8080)")
	targetAddr := flag.String("target", "", "Target address (e.g. localhost:9000)")
	listenCompress := flag.Bool("listen-compress", false, "Enable zstd compression on the listen side")
	remoteCompress := flag.Bool("remote-compress", false, "Enable zstd compression on the remote side")
	compressionLevelint := flag.Int("compression-level", 7, "Compression level (1,3,7,22)")
	flag.Parse()
	if *compressionLevelint < 1 || *compressionLevelint > 22 {
		log.Println("Compression level must be between 1 and 22")
		os.Exit(1)
	}
	if *compressionLevelint == 1 {
		compressionLevel = zstd.SpeedFastest
	} else if *compressionLevelint == 3 {
		compressionLevel = zstd.SpeedDefault
	} else if *compressionLevelint == 7 {
		compressionLevel = zstd.SpeedBetterCompression
	} else if *compressionLevelint == 22 {
		compressionLevel = zstd.SpeedBestCompression
	} else {
		compressionLevel = zstd.SpeedBetterCompression
	}

	if *targetAddr == "" {
		log.Println("Target address must be provided using the -target flag, e.g. -target localhost:1080 -listen :8888")
		os.Exit(1)
	}
	log.Printf("Compression level: %v", compressionLevel)

	log.Printf("Listen compression: %v", *listenCompress)
	log.Printf("Remote compression: %v", *remoteCompress)
	log.Printf("Proxy listening on %s, forwarding to %s", *listenAddr, *targetAddr)

	// Start listening for incoming connections.
	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Error listening on %s: %v", *listenAddr, err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go handleConnection(conn, *targetAddr, *listenCompress, *remoteCompress)
	}
}
