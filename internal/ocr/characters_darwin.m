//go:build darwin && cgo

#import <Vision/Vision.h>
#import <Foundation/Foundation.h>
#import <ImageIO/ImageIO.h>
#include <string.h>

static NSArray *glance_ocr_rect(CGRect r) {
 return @[@(r.origin.x), @(1-r.origin.y-r.size.height), @(r.origin.x+r.size.width), @(1-r.origin.y)];
}

int glance_ocr_characters(const void *data, size_t length, char **output, char **message) {
 @autoreleasepool {
  NSData *pixels=[NSData dataWithBytes:data length:length];
  VNImageRequestHandler *handler=[[VNImageRequestHandler alloc] initWithData:pixels options:@{}];
  VNRecognizeTextRequest *request=[[VNRecognizeTextRequest alloc] init];
  request.recognitionLevel=VNRequestTextRecognitionLevelAccurate;
  request.usesLanguageCorrection=NO;
  request.minimumTextHeight=0.005;
  NSError *error=nil;
  NSArray *supported=[request supportedRecognitionLanguagesAndReturnError:&error];
  NSMutableArray *languages=[NSMutableArray array];
  for(NSString *language in @[@"zh-Hans",@"zh-Hant",@"en-US"]){ if([supported containsObject:language]) [languages addObject:language]; }
  if(languages.count>0) request.recognitionLanguages=languages;
  BOOL success=[handler performRequests:@[request] error:&error];
  if(!success){*message=strdup(error.localizedDescription.UTF8String?:"native OCR failed");[request release];[handler release];return 0;}
  NSMutableArray *lines=[NSMutableArray array];
  for(VNRecognizedTextObservation *observation in request.results){
   VNRecognizedText *candidate=[[observation topCandidates:1] firstObject];
   if(!candidate)continue;
   NSString *text=candidate.string;
   NSMutableArray *characters=[NSMutableArray array];
   for(NSUInteger index=0;index<text.length;){
    NSRange range=[text rangeOfComposedCharacterSequenceAtIndex:index];
    NSString *value=[text substringWithRange:range];
    VNRectangleObservation *box=[candidate boundingBoxForRange:range error:nil];
    if(box && box.boundingBox.size.width>0 && box.boundingBox.size.height>0){
     [characters addObject:@{@"text":value,@"bounds":glance_ocr_rect(box.boundingBox)}];
    }
    index=NSMaxRange(range);
   }
   [lines addObject:@{@"bounds":glance_ocr_rect(observation.boundingBox),@"characters":characters}];
  }
  NSData *json=[NSJSONSerialization dataWithJSONObject:lines options:0 error:&error];
  if(!json){*message=strdup(error.localizedDescription.UTF8String?:"encode native OCR failed");[request release];[handler release];return 0;}
  *output=strndup(json.bytes,json.length);
  [request release];[handler release];
  return 1;
 }
}
