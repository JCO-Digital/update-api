# WordPress Integration Guide

To connect your WordPress plugin to the update API, you can use the following boilerplate.

## Boilerplate Code

Add this to your main plugin file or a dedicated `updates.php` file included by your plugin.

```php
<?php

class MyPluginUpdater {
    private $slug;
    private $current_version;
    private $update_url;
    private $license_key;

    public function __construct($slug, $version, $update_url, $license_key = '') {
        $this->slug = $slug;
        $this->current_version = $version;
        $this->update_url = $update_url;
        $this->license_key = $license_key;

        // Hook into update checks
        add_filter('pre_set_site_transient_update_plugins', [$this, 'check_update']);
        
        // Hook into plugin details popup
        add_filter('plugins_api', [$this, 'plugin_popup'], 20, 3);
    }

    public function check_update($transient) {
        if (empty($transient->checked)) {
            return $transient;
        }

        $remote = $this->request();

        if ($remote && version_compare($this->current_version, $remote->new_version, '<')) {
            $res = new stdClass();
            $res->slug = $this->slug;
            $res->plugin = $this->slug . '/' . $this->slug . '.php'; // Adjust if folder != filename
            $res->new_version = $remote->new_version;
            $res->tested = $remote->tested;
            $res->package = $remote->package;
            $res->url = $remote->url;
            
            $transient->response[$res->plugin] = $res;
        }

        return $transient;
    }

    public function plugin_popup($res, $action, $args) {
        if ($action !== 'plugin_information' || $args->slug !== $this->slug) {
            return $res;
        }

        $remote = $this->request();

        if (!$remote) {
            return $res;
        }

        $res = new stdClass();
        $res->name = 'My Plugin'; // Customize
        $res->slug = $this->slug;
        $res->version = $remote->new_version;
        $res->tested = $remote->tested;
        $res->requires = $remote->requires;
        $res->requires_php = $remote->requires_php;
        $res->download_link = $remote->package;
        $res->sections = (array) $remote->sections;
        $res->last_updated = date('Y-m-d H:i:s');

        return $res;
    }

    private function request() {
        $url = add_query_arg([
            'slug' => $this->slug,
            'version' => $this->current_version,
            'license_key' => $this->license_key
        ], $this->update_url);

        $response = wp_remote_get($url, [
            'timeout' => 10,
            'headers' => ['Accept' => 'application/json']
        ]);

        if (is_wp_error($response) || wp_remote_retrieve_response_code($response) !== 200) {
            return false;
        }

        return json_decode(wp_remote_retrieve_body($response));
    }
}

// Initialize
new MyPluginUpdater('my-plugin', '1.0.0', 'https://api.example.com/v1/update-check', 'KEY-GOES-HERE');
```
