// A fault handler for bringing the HarmonyOS host up: a signal in the core's
// C to Go entry (or back) kills the process with nothing in hilog — the
// runtime's own traceback goes to a fd 2 no one reads — so it's logged here,
// with the faulting instruction's address, to symbolize against the library.
// Development only: it takes Go's own handlers place.

#include <signal.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <ucontext.h>
#include <unistd.h>

#include <hilog/log.h>

#define gokit_LOG_DOMAIN 0x0
#define gokit_LOG_TAG "gokit"

#define logi(...) OH_LOG_Print(LOG_APP, LOG_INFO, gokit_LOG_DOMAIN, gokit_LOG_TAG, __VA_ARGS__)

static void onFault(int sig, siginfo_t *si, void *ctx) {
    ucontext_t *uc = (ucontext_t *)ctx;
    mcontext_t *mc = &uc->uc_mcontext;
    logi("FAULT signal %{public}d at %{public}p, pc %{public}p, lr %{public}p, sp %{public}p",
         sig, si ? si->si_addr : NULL, (void *)mc->pc, (void *)mc->regs[30], (void *)mc->sp);
    _exit(90);
}

void gokit_fault_handler(void) {
    struct sigaction sa;
    memset(&sa, 0, sizeof sa);
    sa.sa_sigaction = onFault;
    sa.sa_flags = SA_SIGINFO;
    sigaction(SIGSEGV, &sa, NULL);
    sigaction(SIGBUS, &sa, NULL);
    sigaction(SIGILL, &sa, NULL);
    sigaction(SIGABRT, &sa, NULL);
    sigaction(SIGTRAP, &sa, NULL);
    sigaction(SIGFPE, &sa, NULL);
}
