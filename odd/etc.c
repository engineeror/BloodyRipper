#include <stddef.h>
#include <stdint.h>

// might be able to do this in go but it's probably pretty akward
// must have this in a separate C file due to the //export preamble restriction https://golang.org/cmd/cgo/#hdr-C_references_to_Go
uint8_t countDrives(char **drives) {
  uint8_t cnt = 0;
  for(; drives[cnt] != NULL; cnt++) {}
  return cnt;
}
