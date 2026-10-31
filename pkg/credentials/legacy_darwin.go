//go:build darwin && cgo

package credentials

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Security -framework CoreFoundation -framework LocalAuthentication -lobjc
#include <Security/Security.h>
#include <LocalAuthentication/LocalAuthentication.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
static OSStatus eos_read_legacy(const char *service, const char *account, void **bytes, long *length) {
 // Legacy login-keychain ACLs also require the process-level noninteractive policy.
 OSStatus policyStatus = SecKeychainSetUserInteractionAllowed(false);
 if(policyStatus != errSecSuccess) return policyStatus;
 CFStringRef svc = CFStringCreateWithCString(NULL,service,kCFStringEncodingUTF8);
 CFStringRef acc = CFStringCreateWithCString(NULL,account,kCFStringEncodingUTF8);
 LAContext *context = [[LAContext alloc] init];
 context.interactionNotAllowed = YES;
 const void *keys[] = {kSecClass,kSecAttrService,kSecAttrAccount,kSecReturnData,kSecUseAuthenticationContext};
 const void *values[] = {kSecClassGenericPassword,svc,acc,kCFBooleanTrue,context};
 CFDictionaryRef query=CFDictionaryCreate(NULL,keys,values,5,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFDataRef data=NULL;
 OSStatus status=SecItemCopyMatching(query,(CFTypeRef*)&data);
 if(status==errSecSuccess){*length=CFDataGetLength(data);*bytes=malloc(*length);if(*length && !*bytes){status=errSecAllocate;}else{CFDataGetBytes(data,CFRangeMake(0,*length),*bytes);}}
 if(data)CFRelease(data);CFRelease(query);CFRelease(svc);CFRelease(acc);[context release];return status;
}
*/
import "C"
import (
	"fmt"
	gokeyring "github.com/zalando/go-keyring"
	"unsafe"
)

func legacyGet(service, account string) (string, error) {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var bytes unsafe.Pointer
	var length C.long
	status := C.eos_read_legacy(s, a, &bytes, &length)
	if status == C.errSecItemNotFound {
		return "", gokeyring.ErrNotFound
	}
	if status != 0 {
		return "", fmt.Errorf("legacy credential status %d", int(status))
	}
	defer C.free(bytes)
	return decodeLegacySecret(C.GoStringN((*C.char)(bytes), C.int(length)))
}
