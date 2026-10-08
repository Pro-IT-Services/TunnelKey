// macOS menu bar item for Tunnelkey (tray_darwin.m). All functions may be
// called from any thread; the work is done on the main thread.
#ifndef TUNNELKEY_TRAY_DARWIN_H
#define TUNNELKEY_TRAY_DARWIN_H

void tkTrayStart(const char *open, const char *connect, const char *quit);
void tkTrayStop(void);
void tkTraySetIcon(const void *png, int length);
void tkTraySetStatus(const char *tooltip, const char *line);
void tkTraySetToggle(const char *title, int enabled);
void tkTraySetLabels(const char *open, const char *quit);

#endif
