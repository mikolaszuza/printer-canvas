// The bundle's entry. The app imports this file and calls what it default
// exports, handing it the one object a module receives.
import { defineModule } from '@prusa/connect-ui/slots';

import { holdHost } from './host';
import Page from './page.svelte';
import './styles.css';

export default defineModule((host) => {
  holdHost(host);
  // Held to `contributes` in module.yaml.
  host.register.page('canvas', Page);
});
