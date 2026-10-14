#ifndef GLANCE_SYSTEM_TRAY_DARWIN_H
#define GLANCE_SYSTEM_TRAY_DARWIN_H

#include <stddef.h>
#include <stdint.h>

int glance_tray_create(uintptr_t tray_id, const void *png_1x, size_t png_1x_len, const void *png_2x, size_t png_2x_len);
int glance_tray_remove(uintptr_t tray_id);
int glance_tray_hide_active_window(void);
int glance_tray_show_active_window(void);

extern void goGlanceTraySetupFinished(uintptr_t tray_id, int success);
extern void goGlanceTrayAction(uintptr_t tray_id, int action);
extern void goGlanceTrayRemoved(uintptr_t tray_id);

#endif
