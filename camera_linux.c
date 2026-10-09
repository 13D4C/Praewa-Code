#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <string.h>

static gint camera_enabled = 0;

void praewa_camera_access(int enabled) {
    g_atomic_int_set(&camera_enabled, enabled);
}

static gboolean camera_permission(WebKitWebView *view, WebKitPermissionRequest *request, gpointer data) {
    if (!WEBKIT_IS_USER_MEDIA_PERMISSION_REQUEST(request)) return FALSE;
    WebKitUserMediaPermissionRequest *media = WEBKIT_USER_MEDIA_PERMISSION_REQUEST(request);
    const gchar *uri = webkit_web_view_get_uri(view);
    gboolean trusted = uri && g_str_has_prefix(uri, "wails://wails/");
    if (!trusted && uri) {
        GUri *parsed = g_uri_parse(uri, G_URI_FLAGS_NONE, NULL);
        if (parsed) {
            const gchar *host = g_uri_get_host(parsed);
            const gchar *scheme = g_uri_get_scheme(parsed);
            trusted = scheme && strcmp(scheme, "http") == 0 && host &&
                (strcmp(host, "localhost") == 0 || strcmp(host, "127.0.0.1") == 0);
            g_uri_unref(parsed);
        }
    }
    if (trusted && g_atomic_int_get(&camera_enabled) &&
        webkit_user_media_permission_is_for_video_device(media) &&
        !webkit_user_media_permission_is_for_display_device(media) &&
        !webkit_user_media_permission_is_for_audio_device(media)) {
        webkit_permission_request_allow(request);
    } else {
        webkit_permission_request_deny(request);
    }
    return TRUE;
}

static void connect_camera(GtkWidget *widget, gpointer data) {
    if (WEBKIT_IS_WEB_VIEW(widget)) {
        if (!g_object_get_data(G_OBJECT(widget), "praewa-camera-permission")) {
            g_signal_connect(widget, "permission-request", G_CALLBACK(camera_permission), NULL);
            g_object_set_data(G_OBJECT(widget), "praewa-camera-permission", GINT_TO_POINTER(1));
            WebKitWebContext *context = webkit_web_view_get_context(WEBKIT_WEB_VIEW(widget));
            webkit_security_manager_register_uri_scheme_as_secure(webkit_web_context_get_security_manager(context), "wails");
            WebKitSettings *settings = webkit_web_view_get_settings(WEBKIT_WEB_VIEW(widget));
            webkit_settings_set_enable_media_stream(settings, TRUE);
            webkit_settings_set_media_playback_allows_inline(settings, TRUE);
        }
    } else if (GTK_IS_CONTAINER(widget)) {
        gtk_container_foreach(GTK_CONTAINER(widget), connect_camera, NULL);
    }
}

static gboolean install_camera(gpointer data) {
    GList *windows = gtk_window_list_toplevels();
    for (GList *item = windows; item; item = item->next) {
        connect_camera(GTK_WIDGET(item->data), NULL);
    }
    g_list_free(windows);
    return G_SOURCE_REMOVE;
}

void praewa_camera_install(void) {
    g_idle_add_full(G_PRIORITY_HIGH, install_camera, NULL, NULL);
}
