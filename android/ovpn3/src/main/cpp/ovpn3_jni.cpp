// JNI bridge between app.tunnelkey.ovpn3.OpenVpnClient (Kotlin) and the
// OpenVPN 3 ClientAPI. Hand-written instead of SWIG so the surface stays small
// and the build does not need extra host tools.
//
// Strings crossing into native code are passed as UTF-8 byte arrays (JNI's
// "modified UTF-8" mangles characters outside the BMP, which matters for
// passwords). Strings going back to Kotlin are decoded with String(byte[], UTF_8).

#include <jni.h>
#include <android/log.h>
#include <sys/socket.h>

#include <memory>
#include <string>
#include <vector>

#include <ovpncli.hpp>

using namespace openvpn;

namespace {

constexpr const char *kTag = "tunnelkey-ovpn3";

JavaVM *g_vm = nullptr;
jclass g_string_class = nullptr;
jmethodID g_string_ctor = nullptr; // String(byte[], String charsetName)
jstring g_utf8_name = nullptr;

// OpenVPN 3 process-wide initialisation (crypto self tests, time base, ...).
ClientAPI::OpenVPNClientHelper *g_helper = nullptr;

// Attaches the current thread to the VM for the lifetime of the object when
// the core calls back from one of its own threads.
class ScopedEnv
{
  public:
    ScopedEnv()
    {
        if (g_vm->GetEnv(reinterpret_cast<void **>(&env_), JNI_VERSION_1_6) == JNI_EDETACHED)
        {
            if (g_vm->AttachCurrentThread(&env_, nullptr) == JNI_OK)
                attached_ = true;
            else
                env_ = nullptr;
        }
    }
    ~ScopedEnv()
    {
        if (attached_)
            g_vm->DetachCurrentThread();
    }
    JNIEnv *get() const { return env_; }
    explicit operator bool() const { return env_ != nullptr; }

  private:
    JNIEnv *env_ = nullptr;
    bool attached_ = false;
};

jstring to_jstring(JNIEnv *env, const std::string &s)
{
    jbyteArray bytes = env->NewByteArray(static_cast<jsize>(s.size()));
    env->SetByteArrayRegion(bytes, 0, static_cast<jsize>(s.size()), reinterpret_cast<const jbyte *>(s.data()));
    auto result = static_cast<jstring>(env->NewObject(g_string_class, g_string_ctor, bytes, g_utf8_name));
    env->DeleteLocalRef(bytes);
    return result;
}

std::string from_bytes(JNIEnv *env, jbyteArray arr)
{
    if (arr == nullptr)
        return {};
    const jsize len = env->GetArrayLength(arr);
    std::string out(static_cast<size_t>(len), '\0');
    env->GetByteArrayRegion(arr, 0, len, reinterpret_cast<jbyte *>(out.data()));
    return out;
}

// Overwrites a std::string's buffer before it is released.
void wipe(std::string &s)
{
    volatile char *p = s.data();
    for (size_t i = 0; i < s.size(); ++i)
        p[i] = 0;
    s.clear();
}

jobjectArray to_string_array(JNIEnv *env, const std::vector<std::string> &items)
{
    jobjectArray arr = env->NewObjectArray(static_cast<jsize>(items.size()), g_string_class, nullptr);
    for (size_t i = 0; i < items.size(); ++i)
    {
        jstring s = to_jstring(env, items[i]);
        env->SetObjectArrayElement(arr, static_cast<jsize>(i), s);
        env->DeleteLocalRef(s);
    }
    return arr;
}

class AndroidClient : public ClientAPI::OpenVPNClient
{
  public:
    AndroidClient(JNIEnv *env, jobject callbacks)
        : callbacks_(env->NewGlobalRef(callbacks))
    {
        jclass cls = env->GetObjectClass(callbacks);
        on_event_ = env->GetMethodID(cls, "onEvent", "(Ljava/lang/String;Ljava/lang/String;ZZ)V");
        on_log_ = env->GetMethodID(cls, "onLog", "(Ljava/lang/String;)V");
        protect_ = env->GetMethodID(cls, "protectSocket", "(I)Z");
        tun_new_ = env->GetMethodID(cls, "tunNew", "()Z");
        tun_add_address_ = env->GetMethodID(cls, "tunAddAddress", "(Ljava/lang/String;IZ)Z");
        tun_reroute_gw_ = env->GetMethodID(cls, "tunRerouteGateway", "(ZZ)Z");
        tun_add_route_ = env->GetMethodID(cls, "tunAddRoute", "(Ljava/lang/String;IZ)Z");
        tun_exclude_route_ = env->GetMethodID(cls, "tunExcludeRoute", "(Ljava/lang/String;IZ)Z");
        tun_add_dns_ = env->GetMethodID(cls, "tunAddDnsServer", "(Ljava/lang/String;)Z");
        tun_add_search_domain_ = env->GetMethodID(cls, "tunAddSearchDomain", "(Ljava/lang/String;)Z");
        tun_set_mtu_ = env->GetMethodID(cls, "tunSetMtu", "(I)Z");
        tun_set_session_name_ = env->GetMethodID(cls, "tunSetSessionName", "(Ljava/lang/String;)Z");
        tun_set_allow_family_ = env->GetMethodID(cls, "tunSetAllowFamily", "(ZZ)Z");
        tun_establish_ = env->GetMethodID(cls, "tunEstablish", "()I");
        tun_teardown_ = env->GetMethodID(cls, "tunTeardown", "(Z)V");
        env->DeleteLocalRef(cls);
    }

    ~AndroidClient() override
    {
        ScopedEnv env;
        if (env)
            env.get()->DeleteGlobalRef(callbacks_);
    }

    // ---- ClientAPI callbacks -------------------------------------------

    void event(const ClientAPI::Event &ev) override
    {
        ScopedEnv env;
        if (!env)
            return;
        JNIEnv *e = env.get();
        jstring name = to_jstring(e, ev.name);
        jstring info = to_jstring(e, ev.info);
        e->CallVoidMethod(callbacks_, on_event_, name, info, ev.error, ev.fatal);
        clear_exception(e);
        e->DeleteLocalRef(name);
        e->DeleteLocalRef(info);
    }

    void acc_event(const ClientAPI::AppCustomControlMessageEvent &) override
    {
    }

    void log(const ClientAPI::LogInfo &info) override
    {
        ScopedEnv env;
        if (!env)
            return;
        JNIEnv *e = env.get();
        jstring text = to_jstring(e, info.text);
        e->CallVoidMethod(callbacks_, on_log_, text);
        clear_exception(e);
        e->DeleteLocalRef(text);
    }

    void external_pki_cert_request(ClientAPI::ExternalPKICertRequest &req) override
    {
        req.error = true;
        req.errorText = "External PKI is not supported";
    }

    void external_pki_sign_request(ClientAPI::ExternalPKISignRequest &req) override
    {
        req.error = true;
        req.errorText = "External PKI is not supported";
    }

    bool pause_on_connection_timeout() override
    {
        return false;
    }

    bool socket_protect(openvpn_io::detail::socket_type socket, std::string, bool) override
    {
        return call_bool(protect_, static_cast<jint>(socket));
    }

    // ---- TunBuilderBase ------------------------------------------------

    bool tun_builder_new() override
    {
        return call_bool(tun_new_);
    }

    bool tun_builder_set_layer(int layer) override
    {
        return layer == 3; // Android VpnService only supports routed (tun) mode
    }

    bool tun_builder_set_remote_address(const std::string &, bool) override
    {
        return true;
    }

    bool tun_builder_add_address(const std::string &address,
                                 int prefix_length,
                                 const std::string &,
                                 bool ipv6,
                                 bool) override
    {
        return call_bool_str(tun_add_address_, address, prefix_length, ipv6);
    }

    bool tun_builder_set_route_metric_default(int) override
    {
        return true;
    }

    bool tun_builder_reroute_gw(bool ipv4, bool ipv6, unsigned int) override
    {
        return call_bool(tun_reroute_gw_, static_cast<jboolean>(ipv4), static_cast<jboolean>(ipv6));
    }

    bool tun_builder_add_route(const std::string &address, int prefix_length, int, bool ipv6) override
    {
        return call_bool_str(tun_add_route_, address, prefix_length, ipv6);
    }

    bool tun_builder_exclude_route(const std::string &address, int prefix_length, int, bool ipv6) override
    {
        return call_bool_str(tun_exclude_route_, address, prefix_length, ipv6);
    }

    bool tun_builder_set_dns_options(const DnsOptions &dns) override
    {
        ScopedEnv env;
        if (!env)
            return false;
        JNIEnv *e = env.get();
        bool ok = true;
        // std::map is ordered by priority, which is also the order to hand to Android.
        for (const auto &[priority, server] : dns.servers)
        {
            for (const auto &addr : server.addresses)
            {
                jstring s = to_jstring(e, addr.address);
                ok = e->CallBooleanMethod(callbacks_, tun_add_dns_, s) && ok;
                clear_exception(e);
                e->DeleteLocalRef(s);
            }
            for (const auto &domain : server.domains)
            {
                jstring s = to_jstring(e, domain.domain);
                e->CallBooleanMethod(callbacks_, tun_add_search_domain_, s);
                clear_exception(e);
                e->DeleteLocalRef(s);
            }
        }
        for (const auto &domain : dns.search_domains)
        {
            jstring s = to_jstring(e, domain.domain);
            e->CallBooleanMethod(callbacks_, tun_add_search_domain_, s);
            clear_exception(e);
            e->DeleteLocalRef(s);
        }
        return ok;
    }

    bool tun_builder_set_mtu(int mtu) override
    {
        return call_bool(tun_set_mtu_, static_cast<jint>(mtu));
    }

    bool tun_builder_set_session_name(const std::string &name) override
    {
        ScopedEnv env;
        if (!env)
            return false;
        jstring s = to_jstring(env.get(), name);
        const bool ok = env.get()->CallBooleanMethod(callbacks_, tun_set_session_name_, s);
        clear_exception(env.get());
        env.get()->DeleteLocalRef(s);
        return ok;
    }

    bool tun_builder_add_proxy_bypass(const std::string &) override
    {
        return true;
    }
    bool tun_builder_set_proxy_auto_config_url(const std::string &) override
    {
        return true;
    }
    bool tun_builder_set_proxy_http(const std::string &, int) override
    {
        return true;
    }
    bool tun_builder_set_proxy_https(const std::string &, int) override
    {
        return true;
    }
    bool tun_builder_add_wins_server(const std::string &) override
    {
        return true;
    }

    bool tun_builder_set_allow_family(int af, bool allow) override
    {
        return call_bool(tun_set_allow_family_, static_cast<jboolean>(af == AF_INET6), static_cast<jboolean>(allow));
    }

    bool tun_builder_set_allow_local_dns(bool) override
    {
        return true;
    }

    int tun_builder_establish() override
    {
        ScopedEnv env;
        if (!env)
            return -1;
        const jint fd = env.get()->CallIntMethod(callbacks_, tun_establish_);
        if (clear_exception(env.get()))
            return -1;
        return fd;
    }

    bool tun_builder_persist() override
    {
        return true;
    }

    std::vector<std::string> tun_builder_get_local_networks(bool) override
    {
        return {};
    }

    void tun_builder_establish_lite() override
    {
    }

    void tun_builder_teardown(bool disconnect) override
    {
        ScopedEnv env;
        if (!env)
            return;
        env.get()->CallVoidMethod(callbacks_, tun_teardown_, static_cast<jboolean>(disconnect));
        clear_exception(env.get());
    }

  private:
    static bool clear_exception(JNIEnv *e)
    {
        if (e->ExceptionCheck())
        {
            e->ExceptionDescribe();
            e->ExceptionClear();
            return true;
        }
        return false;
    }

    template <typename... Args>
    bool call_bool(jmethodID method, Args... args)
    {
        ScopedEnv env;
        if (!env)
            return false;
        const bool ok = env.get()->CallBooleanMethod(callbacks_, method, args...);
        return !clear_exception(env.get()) && ok;
    }

    bool call_bool_str(jmethodID method, const std::string &address, int prefix, bool ipv6)
    {
        ScopedEnv env;
        if (!env)
            return false;
        jstring s = to_jstring(env.get(), address);
        const bool ok = env.get()->CallBooleanMethod(callbacks_, method, s, static_cast<jint>(prefix), static_cast<jboolean>(ipv6));
        const bool threw = clear_exception(env.get());
        env.get()->DeleteLocalRef(s);
        return !threw && ok;
    }

    jobject callbacks_;
    jmethodID on_event_, on_log_, protect_;
    jmethodID tun_new_, tun_add_address_, tun_reroute_gw_, tun_add_route_, tun_exclude_route_;
    jmethodID tun_add_dns_, tun_add_search_domain_, tun_set_mtu_, tun_set_session_name_;
    jmethodID tun_set_allow_family_, tun_establish_, tun_teardown_;
};

AndroidClient *client_from(jlong handle)
{
    return reinterpret_cast<AndroidClient *>(handle);
}

} // namespace

extern "C" {

JNIEXPORT jint JNICALL JNI_OnLoad(JavaVM *vm, void *)
{
    g_vm = vm;
    JNIEnv *env = nullptr;
    if (vm->GetEnv(reinterpret_cast<void **>(&env), JNI_VERSION_1_6) != JNI_OK)
        return JNI_ERR;

    jclass local = env->FindClass("java/lang/String");
    g_string_class = static_cast<jclass>(env->NewGlobalRef(local));
    env->DeleteLocalRef(local);
    g_string_ctor = env->GetMethodID(g_string_class, "<init>", "([BLjava/lang/String;)V");
    jstring utf8 = env->NewStringUTF("UTF-8");
    g_utf8_name = static_cast<jstring>(env->NewGlobalRef(utf8));
    env->DeleteLocalRef(utf8);

    g_helper = new ClientAPI::OpenVPNClientHelper();
    __android_log_print(ANDROID_LOG_INFO, kTag, "%s", ClientAPI::OpenVPNClientHelper::platform().c_str());
    return JNI_VERSION_1_6;
}

JNIEXPORT jlong JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeCreate(JNIEnv *env, jclass, jobject callbacks)
{
    return reinterpret_cast<jlong>(new AndroidClient(env, callbacks));
}

JNIEXPORT void JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeDestroy(JNIEnv *, jclass, jlong handle)
{
    delete client_from(handle);
}

// Returns [error, message, autologin, staticChallenge, staticChallengeEcho,
//          profileName, remoteHost, remotePort, remoteProto]
JNIEXPORT jobjectArray JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeEval(JNIEnv *env,
                                                  jclass,
                                                  jlong handle,
                                                  jbyteArray content,
                                                  jbyteArray gui_version,
                                                  jint conn_timeout,
                                                  jboolean allow_lan)
{
    ClientAPI::Config config;
    config.content = from_bytes(env, content);
    config.guiVersion = from_bytes(env, gui_version);
    config.platformVersion = "android";
    config.tunPersist = true;
    config.connTimeout = conn_timeout;
    config.allowLocalLanAccess = allow_lan;
    config.info = true;
    config.autologinSessions = true;

    const ClientAPI::EvalConfig eval = client_from(handle)->eval_config(config);
    wipe(config.content);
    return to_string_array(env,
                           {eval.error ? "1" : "0",
                            eval.message,
                            eval.autologin ? "1" : "0",
                            eval.staticChallenge,
                            eval.staticChallengeEcho ? "1" : "0",
                            eval.profileName,
                            eval.remoteHost,
                            eval.remotePort,
                            eval.remoteProto});
}

// Returns null on success, otherwise an error message.
JNIEXPORT jstring JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeProvideCreds(JNIEnv *env,
                                                          jclass,
                                                          jlong handle,
                                                          jbyteArray username,
                                                          jbyteArray password,
                                                          jbyteArray response)
{
    ClientAPI::ProvideCreds creds;
    creds.username = from_bytes(env, username);
    creds.password = from_bytes(env, password);
    creds.response = from_bytes(env, response);
    const ClientAPI::Status status = client_from(handle)->provide_creds(creds);
    wipe(creds.password);
    wipe(creds.response);
    if (!status.error)
        return nullptr;
    return to_jstring(env, status.message.empty() ? status.status : status.message);
}

// Blocks until the session ends. Returns null on a clean stop, otherwise
// "status\nmessage".
JNIEXPORT jstring JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeConnect(JNIEnv *env, jclass, jlong handle)
{
    const ClientAPI::Status status = client_from(handle)->connect();
    if (!status.error)
        return nullptr;
    return to_jstring(env, status.status + "\n" + status.message);
}

JNIEXPORT void JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeStop(JNIEnv *, jclass, jlong handle)
{
    client_from(handle)->stop();
}

JNIEXPORT void JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeReconnect(JNIEnv *, jclass, jlong handle, jint seconds)
{
    client_from(handle)->reconnect(seconds);
}

JNIEXPORT void JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativePause(JNIEnv *env, jclass, jlong handle, jbyteArray reason)
{
    client_from(handle)->pause(from_bytes(env, reason));
}

JNIEXPORT void JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeResume(JNIEnv *, jclass, jlong handle)
{
    client_from(handle)->resume();
}

// Returns [bytesIn, bytesOut]
JNIEXPORT jlongArray JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeTransportStats(JNIEnv *env, jclass, jlong handle)
{
    const ClientAPI::TransportStats stats = client_from(handle)->transport_stats();
    const jlong values[2] = {stats.bytesIn, stats.bytesOut};
    jlongArray arr = env->NewLongArray(2);
    env->SetLongArrayRegion(arr, 0, 2, values);
    return arr;
}

// Returns [vpnIp4, vpnIp6, serverHost, serverIp, serverPort, serverProto, user]
JNIEXPORT jobjectArray JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeConnectionInfo(JNIEnv *env, jclass, jlong handle)
{
    const ClientAPI::ConnectionInfo ci = client_from(handle)->connection_info();
    return to_string_array(env,
                           {ci.vpnIp4, ci.vpnIp6, ci.serverHost, ci.serverIp,
                            ci.serverPort, ci.serverProto, ci.user});
}

JNIEXPORT jstring JNICALL
Java_app_tunnelkey_ovpn3_OpenVpnClient_nativeCoreVersion(JNIEnv *env, jclass)
{
    return to_jstring(env, ClientAPI::OpenVPNClientHelper::platform());
}

} // extern "C"
