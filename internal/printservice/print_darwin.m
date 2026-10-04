#import <AppKit/AppKit.h>
#import <PDFKit/PDFKit.h>
#import <dispatch/dispatch.h>
#include <stdlib.h>
#include <string.h>

enum {
	GlancePrintSucceeded = 0,
	GlancePrintCanceled = 1,
	GlancePrintFailed = 2,
};

struct GlancePrintJob {
	volatile int done;
	volatile int outcome;
	char *error_message;
	void *pdf_bytes;
	size_t pdf_length;
	int copies;
	int first_page;
	int last_page;
};

static void glance_print_complete(struct GlancePrintJob *job, int outcome, NSString *message) {
	if (message.length > 0) {
		const char *utf8 = message.UTF8String;
		if (utf8 != NULL) {
			job->error_message = strdup(utf8);
		}
	}
	__atomic_store_n(&job->outcome, outcome, __ATOMIC_RELAXED);
	__atomic_store_n(&job->done, 1, __ATOMIC_RELEASE);
}

static void glance_print_run(struct GlancePrintJob *job) {
	@autoreleasepool {
		@try {
			NSData *data = [NSData dataWithBytes:job->pdf_bytes length:job->pdf_length];
			PDFDocument *document = [[PDFDocument alloc] initWithData:data];
			if (document == nil || document.pageCount == 0) {
				glance_print_complete(job, GlancePrintFailed, @"PDFKit could not read a printable PDF document.");
				return;
			}

			NSMutableDictionary *settings = [[[NSPrintInfo sharedPrintInfo] dictionary] mutableCopy];
			settings[NSPrintCopies] = @(MAX(1, job->copies));
			if (job->first_page > 0 && job->last_page >= job->first_page) {
				settings[NSPrintAllPages] = @NO;
				settings[NSPrintFirstPage] = @(job->first_page);
				settings[NSPrintLastPage] = @(job->last_page);
			} else {
				settings[NSPrintAllPages] = @YES;
			}
			NSPrintInfo *printInfo = [[NSPrintInfo alloc] initWithDictionary:settings];
			NSPrintOperation *operation = [document printOperationForPrintInfo:printInfo scalingMode:kPDFPrintPageScaleNone autoRotate:NO];
			if (operation == nil) {
				glance_print_complete(job, GlancePrintFailed, @"PDFKit could not create a native print operation.");
				return;
			}
			operation.showsPrintPanel = YES;
			operation.showsProgressPanel = YES;
			BOOL submitted = [operation runOperation];
			glance_print_complete(job, submitted ? GlancePrintSucceeded : GlancePrintCanceled, nil);
		} @catch (NSException *exception) {
			glance_print_complete(job, GlancePrintFailed, exception.reason ?: @"Native print operation raised an exception.");
		}
	}
}

struct GlancePrintJob *glance_print_pdf_start(const void *data, size_t length, int copies, int first_page, int last_page) {
	if (data == NULL || length == 0) {
		return NULL;
	}
	struct GlancePrintJob *job = calloc(1, sizeof(*job));
	if (job == NULL) {
		return NULL;
	}
	job->pdf_bytes = malloc(length);
	if (job->pdf_bytes == NULL) {
		free(job);
		return NULL;
	}
	memcpy(job->pdf_bytes, data, length);
	job->pdf_length = length;
	job->copies = copies;
	job->first_page = first_page;
	job->last_page = last_page;
	if ([NSThread isMainThread]) {
		glance_print_run(job);
	} else {
		dispatch_async(dispatch_get_main_queue(), ^{
			glance_print_run(job);
		});
	}
	return job;
}

int glance_print_pdf_done(struct GlancePrintJob *job) {
	return job != NULL && __atomic_load_n(&job->done, __ATOMIC_ACQUIRE) != 0;
}

int glance_print_pdf_outcome(struct GlancePrintJob *job) {
	return job == NULL ? GlancePrintFailed : __atomic_load_n(&job->outcome, __ATOMIC_ACQUIRE);
}

const char *glance_print_pdf_error(struct GlancePrintJob *job) {
	return job == NULL ? NULL : job->error_message;
}

void glance_print_pdf_free(struct GlancePrintJob *job) {
	if (job == NULL) {
		return;
	}
	free(job->pdf_bytes);
	free(job->error_message);
	free(job);
}
