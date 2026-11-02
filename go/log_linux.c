// The app's output, in hilog: HarmonyOS gives a process's fds 1 and 2 to no
// one, so what the core prints, and what the runtime writes on its way out (a
// panic's traceback, a fatal error, a signal's) is lost. Here the fds go to a
// pipe a thread of their own reads into hilog, tagged gokit. It's a C thread,
// not a goroutine: the runtime exits the process right after it writes, and
// only a thread already blocked in read(2) gets there first.
//
//   hdc shell hilog -T gokit

#include <errno.h>
#include <pthread.h>
#include <stdint.h>
#include <unistd.h>

#include <hilog/log.h>

#define gokit_LOG_DOMAIN 0x0
#define gokit_LOG_TAG "gokit"

static void *gokitLogReader(void *arg) {
    int fd = (int)(intptr_t)arg;
    char buf[1024];
    for (;;) {
        ssize_t n = read(fd, buf, sizeof buf - 1);
        if (n < 0) {
            if (errno == EINTR) continue;
            break;
        }
        if (n == 0) break;
        while (n > 0 && (buf[n - 1] == '\n' || buf[n - 1] == '\r')) n--;
        buf[n] = 0;
        if (n == 0) continue;
        OH_LOG_Print(LOG_APP, LOG_WARN, gokit_LOG_DOMAIN, gokit_LOG_TAG, "%{public}s", buf);
    }
    return NULL;
}

// A constructor, before the core's own: what it prints while starting is the
// most interesting output there is.
__attribute__((constructor)) static void gokitLogStart(void) {
    int fds[2];
    if (pipe(fds) != 0) return;
    if (dup2(fds[1], 1) < 0 || dup2(fds[1], 2) < 0) {
        close(fds[0]);
        close(fds[1]);
        return;
    }
    close(fds[1]);
    pthread_t t;
    if (pthread_create(&t, NULL, gokitLogReader, (void *)(intptr_t)fds[0]) == 0) {
        pthread_detach(t);
    } else {
        close(fds[0]);
    }
}
