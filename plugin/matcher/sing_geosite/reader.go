package sing_geosite

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync/atomic"
)

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
}

type Reader struct {
	reader       ReadSeekCloser
	domainIndex  map[string]int
	domainLength map[string]int
}

func OpenGeoSiteReader(path string) (*Reader, []string, error) {
	content, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	reader := &Reader{
		reader: content,
	}
	err = reader.readMetadata()
	if err != nil {
		content.Close()
		return nil, nil, err
	}
	codes := make([]string, 0, len(reader.domainIndex))
	for code := range reader.domainIndex {
		codes = append(codes, code)
	}
	return reader, codes, nil
}

func (r *Reader) readMetadata() error {
	version, err := readByte(r.reader)
	if err != nil {
		return err
	}
	if version != 0 {
		return fmt.Errorf("unknown version")
	}
	entryLength, err := readUVariant(r.reader)
	if err != nil {
		return err
	}
	keys := make([]string, entryLength)
	domainIndex := make(map[string]int)
	domainLength := make(map[string]int)
	for i := 0; i < int(entryLength); i++ {
		var (
			code       string
			codeIndex  uint64
			codeLength uint64
		)
		code, err = readVString(r.reader)
		if err != nil {
			return err
		}
		keys[i] = code
		codeIndex, err = readUVariant(r.reader)
		if err != nil {
			return err
		}
		codeLength, err = readUVariant(r.reader)
		if err != nil {
			return err
		}
		domainIndex[code] = int(codeIndex)
		domainLength[code] = int(codeLength)
	}
	r.domainIndex = domainIndex
	r.domainLength = domainLength
	return nil
}

func (r *Reader) Read(code string) ([]Item, error) {
	index, exists := r.domainIndex[code]
	if !exists {
		return nil, fmt.Errorf("code [%s] not exists", code)
	}
	_, err := r.reader.Seek(int64(index), io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	counter := &readCounter{Reader: r.reader}
	domain := make([]Item, r.domainLength[code])
	for i := range domain {
		var (
			item Item
			err  error
		)
		item.Type, err = readByte(counter)
		if err != nil {
			return nil, err
		}
		item.Value, err = readVString(counter)
		if err != nil {
			return nil, err
		}
		domain[i] = item
	}
	_, err = r.reader.Seek(int64(-index)-counter.Count(), io.SeekCurrent)
	return domain, err
}

func (r *Reader) Close() error {
	return r.reader.Close()
}

type readCounter struct {
	io.Reader
	count int64
}

func (r *readCounter) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.count, int64(n))
	}
	return
}

func (r *readCounter) Count() int64 {
	return r.count
}

func (r *readCounter) Reset() {
	atomic.StoreInt64(&r.count, 0)
}

func readByte(reader io.Reader) (byte, error) {
	if br, isBr := reader.(io.ByteReader); isBr {
		return br.ReadByte()
	}
	var b [1]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

type stubByteReader struct {
	io.Reader
}

func (r stubByteReader) ReadByte() (byte, error) {
	return readByte(r.Reader)
}

func toByteReader(reader io.Reader) io.ByteReader {
	if byteReader, ok := reader.(io.ByteReader); ok {
		return byteReader
	}
	return &stubByteReader{reader}
}

func readUVariant(reader io.Reader) (uint64, error) {
	return binary.ReadUvarint(toByteReader(reader))
}

func readBytes(reader io.Reader, size int) ([]byte, error) {
	b := make([]byte, size)
	_, err := io.ReadFull(reader, b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func readVString(reader io.Reader) (string, error) {
	length, err := binary.ReadUvarint(toByteReader(reader))
	if err != nil {
		return "", err
	}
	value, err := readBytes(reader, int(length))
	if err != nil {
		return "", err
	}
	return string(value), nil
}
