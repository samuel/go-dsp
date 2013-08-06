#include "exports.h"

void cbAsync(unsigned char *buf, uint32_t len, void *ctx) {
	cbAsyncGo(buf, len, ctx);
}

const rtlsdr_read_async_cb_t *cbAsyncPtr = (rtlsdr_read_async_cb_t*)&cbAsync;
