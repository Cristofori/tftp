package filestore

import "sync"

var files map[string][]byte
var mutex sync.RWMutex

func Init() {
	files = map[string][]byte{}
}

func Exists(filename string) bool {
	mutex.RLock()
	defer mutex.RUnlock()

	_, found := files[filename]
	return found
}

func Create(filename string, data []byte) bool {
	if Exists(filename) {
		return false
	}

	mutex.Lock()
	defer mutex.Unlock()

	files[filename] = data
	return true
}

func Get(filename string) ([]byte, bool) {
	mutex.RLock()
	defer mutex.RUnlock()

	file, found := files[filename]

	return file, found
}
