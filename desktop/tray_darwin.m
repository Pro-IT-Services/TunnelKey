// macOS menu bar item for Tunnelkey.
//
// Written directly against AppKit instead of using a tray library: those
// replace the NSApplication delegate, which Wails needs for its window, Quit
// and "open file". This only adds an NSStatusItem to the app Wails runs.
// Compiled without ARC (cgo default), so ownership is managed by hand.

#import <Cocoa/Cocoa.h>
#include "tray_darwin.h"

extern void tunnelkeyTrayReady(void);
extern void tunnelkeyTrayClicked(int item);

@interface TKTrayTarget : NSObject
- (void)clicked:(NSMenuItem *)sender;
@end

@implementation TKTrayTarget
- (void)clicked:(NSMenuItem *)sender {
	tunnelkeyTrayClicked((int)sender.tag);
}
@end

static NSStatusItem *tkItem;
static NSMenu *tkMenu;
static NSMenuItem *tkStatus, *tkOpen, *tkToggle, *tkQuit;
static TKTrayTarget *tkTarget;

static NSString *tkString(const char *s) {
	return [NSString stringWithUTF8String:(s ? s : "")];
}

static NSMenuItem *tkAddItem(NSString *title, int tag, BOOL actionable) {
	NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:title
	                                              action:(actionable ? @selector(clicked:) : nil)
	                                       keyEquivalent:@""];
	item.target = actionable ? tkTarget : nil;
	item.tag = tag;
	item.enabled = actionable;
	[tkMenu addItem:item];
	return item; // owned by the static variable it's assigned to
}

void tkTrayStart(const char *open, const char *connect, const char *quit) {
	@autoreleasepool {
		NSString *o = [tkString(open) retain];
		NSString *c = [tkString(connect) retain];
		NSString *q = [tkString(quit) retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			if (tkItem == nil) {
				tkTarget = [[TKTrayTarget alloc] init];
				tkItem = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength] retain];
				tkMenu = [[NSMenu alloc] initWithTitle:@"Tunnelkey"];
				tkMenu.autoenablesItems = NO;
				tkStatus = tkAddItem(@"Tunnelkey", 0, NO);
				[tkMenu addItem:[NSMenuItem separatorItem]];
				tkOpen = tkAddItem(o, 1, YES);
				tkToggle = tkAddItem(c, 2, YES);
				[tkMenu addItem:[NSMenuItem separatorItem]];
				tkQuit = tkAddItem(q, 3, YES);
				tkItem.menu = tkMenu;
				tkItem.button.toolTip = @"Tunnelkey";
			}
			[o release];
			[c release];
			[q release];
			tunnelkeyTrayReady();
		});
	}
}

void tkTrayStop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (tkItem != nil) {
			[[NSStatusBar systemStatusBar] removeStatusItem:tkItem];
			[tkItem release];
			tkItem = nil;
		}
	});
}

void tkTraySetIcon(const void *png, int length) {
	@autoreleasepool {
		NSData *data = [[NSData alloc] initWithBytes:png length:(NSUInteger)length]; // copied
		dispatch_async(dispatch_get_main_queue(), ^{
			NSImage *image = [[NSImage alloc] initWithData:data];
			if (image != nil && tkItem != nil) {
				image.size = NSMakeSize(18, 18); // menu bar height
				tkItem.button.image = image;
			}
			[image release];
			[data release];
		});
	}
}

void tkTraySetStatus(const char *tooltip, const char *line) {
	@autoreleasepool {
		NSString *t = [tkString(tooltip) retain];
		NSString *l = [tkString(line) retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			if (tkItem != nil) {
				tkItem.button.toolTip = t;
				tkStatus.title = l;
			}
			[t release];
			[l release];
		});
	}
}

void tkTraySetToggle(const char *title, int enabled) {
	@autoreleasepool {
		NSString *t = [tkString(title) retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			if (tkToggle != nil) {
				tkToggle.title = t;
				tkToggle.enabled = enabled ? YES : NO;
			}
			[t release];
		});
	}
}

void tkTraySetLabels(const char *open, const char *quit) {
	@autoreleasepool {
		NSString *o = [tkString(open) retain];
		NSString *q = [tkString(quit) retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			if (tkOpen != nil) {
				tkOpen.title = o;
				tkQuit.title = q;
			}
			[o release];
			[q release];
		});
	}
}
