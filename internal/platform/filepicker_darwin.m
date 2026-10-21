#import <Cocoa/Cocoa.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#include <stdlib.h>
#include <string.h>

// PickFile shows the open panel and returns the chosen path, which the
// caller frees, or NULL when the user cancels. types is a comma-separated
// list of file name extensions; empty means any file.
char *PickFile(const char *prompt, const char *types, int folders) {
  @autoreleasepool {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.message = [NSString stringWithUTF8String:prompt];
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = folders != 0;
    panel.allowsMultipleSelection = NO;
    NSMutableArray<UTType *> *allowed = [NSMutableArray array];
    for (NSString *extension in [[NSString stringWithUTF8String:types] componentsSeparatedByString:@","]) {
      UTType *type = extension.length > 0 ? [UTType typeWithFilenameExtension:extension] : nil;
      if (type != nil) [allowed addObject:type];
    }
    if (allowed.count > 0) {
      if (folders) [allowed addObject:UTTypeFolder];
      panel.allowedContentTypes = allowed;
    }
    if ([panel runModal] != NSModalResponseOK || panel.URL == nil) return NULL;
    return strdup(panel.URL.path.fileSystemRepresentation);
  }
}
