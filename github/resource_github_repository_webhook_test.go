package github

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v31/github"
	"github.com/hashicorp/terraform-plugin-sdk/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func TestAccGithubRepositoryWebhook_basic(t *testing.T) {
	rn := "github_repository_webhook.foo"
	randString := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	var hook github.Hook

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckGithubRepositoryWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGithubRepositoryWebhookConfig(randString),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGithubRepositoryWebhookExists(rn, fmt.Sprintf("foo-%s", randString), &hook),
					testAccCheckGithubRepositoryWebhookAttributes(&hook, &testAccGithubRepositoryWebhookExpectedAttributes{
						Events: []string{"pull_request"},
						Configuration: map[string]interface{}{
							"url":          "https://google.de/webhook",
							"content_type": "json",
							"insecure_ssl": "1",
						},
						Active: true,
					}),
				),
			},
			{
				Config: testAccGithubRepositoryWebhookUpdateConfig(randString),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGithubRepositoryWebhookExists(rn, fmt.Sprintf("foo-%s", randString), &hook),
					testAccCheckGithubRepositoryWebhookAttributes(&hook, &testAccGithubRepositoryWebhookExpectedAttributes{
						Events: []string{"issues"},
						Configuration: map[string]interface{}{
							"url":          "https://google.de/webhooks",
							"content_type": "form",
							"insecure_ssl": "0",
						},
						Active: false,
					}),
				),
			},
			{
				ResourceName:        rn,
				ImportState:         true,
				ImportStateVerify:   true,
				ImportStateIdPrefix: fmt.Sprintf("foo-%s/", randString),
			},
		},
	})
}

func TestAccGithubRepositoryWebhook_secret(t *testing.T) {
	rn := "github_repository_webhook.foo"
	randString := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	var hook github.Hook

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckGithubRepositoryWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccGithubRepositoryWebhookConfig_secret(randString),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckGithubRepositoryWebhookExists(rn, fmt.Sprintf("foo-%s", randString), &hook),
					testAccCheckGithubRepositoryWebhookAttributes(&hook, &testAccGithubRepositoryWebhookExpectedAttributes{
						Events: []string{"pull_request"},
						Configuration: map[string]interface{}{
							"url":          "https://www.terraform.io/webhook",
							"content_type": "json",
							"secret":       "********",
							"insecure_ssl": "0",
						},
						Active: true,
					}),
				),
			},
			{
				ResourceName:            rn,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdPrefix:     fmt.Sprintf("foo-%s/", randString),
				ImportStateVerifyIgnore: []string{"configuration.0.secret"},
			},
		},
	})
}

func testAccCheckGithubRepositoryWebhookExists(n string, repoName string, hook *github.Hook) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not Found: %s", n)
		}

		hookID, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return unconvertibleIdErr(rs.Primary.ID, err)
		}
		if hookID == 0 {
			return fmt.Errorf("No repository name is set")
		}

		org := testAccProvider.Meta().(*Organization)
		conn := org.v3client
		getHook, _, err := conn.Repositories.GetHook(context.TODO(), org.name, repoName, hookID)
		if err != nil {
			return err
		}
		*hook = *getHook
		return nil
	}
}

type testAccGithubRepositoryWebhookExpectedAttributes struct {
	Events        []string
	Configuration map[string]interface{}
	Active        bool
}

func testAccCheckGithubRepositoryWebhookAttributes(hook *github.Hook, want *testAccGithubRepositoryWebhookExpectedAttributes) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if *hook.Active != want.Active {
			return fmt.Errorf("got hook %t; want %t", *hook.Active, want.Active)
		}
		if !strings.HasPrefix(*hook.URL, "https://") {
			return fmt.Errorf("got http URL %q; want to start with 'https://'", *hook.URL)
		}
		if !reflect.DeepEqual(hook.Events, want.Events) {
			return fmt.Errorf("got hook events %q; want %q", hook.Events, want.Events)
		}
		if !reflect.DeepEqual(hook.Config, want.Configuration) {
			return fmt.Errorf("got hook configuration %q; want %q", hook.Config, want.Configuration)
		}

		return nil
	}
}

func testAccCheckGithubRepositoryWebhookDestroy(s *terraform.State) error {
	conn := testAccProvider.Meta().(*Organization).v3client
	orgName := testAccProvider.Meta().(*Organization).name

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "github_repository_webhook" {
			continue
		}

		id, err := strconv.ParseInt(rs.Primary.ID, 10, 64)
		if err != nil {
			return unconvertibleIdErr(rs.Primary.ID, err)
		}

		gotHook, resp, err := conn.Repositories.GetHook(context.TODO(), orgName, rs.Primary.Attributes["repository"], id)
		if err == nil {
			if gotHook != nil && gotHook.GetID() == id {
				return fmt.Errorf("Webhook still exists")
			}
		}
		if resp.StatusCode != 404 {
			return err
		}
		return nil
	}
	return nil
}

func testAccGithubRepositoryWebhookConfig(randString string) string {
	return fmt.Sprintf(`
resource "github_repository" "foo" {
  name         = "foo-%s"
  description  = "Terraform acceptance tests"
  homepage_url = "http://example.com/"

  # So that acceptance tests can be run in a github organization
  # with no billing
  private = false

  has_issues    = true
  has_wiki      = true
  has_downloads = true
}

resource "github_repository_webhook" "foo" {
  depends_on = ["github_repository.foo"]
  repository = "foo-%s"

  configuration {
    url          = "https://google.de/webhook"
    content_type = "json"
    insecure_ssl = true
  }

  events = ["pull_request"]
}
`, randString, randString)
}

func testAccGithubRepositoryWebhookConfig_secret(randString string) string {
	return fmt.Sprintf(`
resource "github_repository" "foo" {
  name         = "foo-%s"
  description  = "Terraform acceptance tests"
  homepage_url = "http://example.com/"

  # So that acceptance tests can be run in a github organization
  # with no billing
  private = false

  has_issues    = true
  has_wiki      = true
  has_downloads = true
}

resource "github_repository_webhook" "foo" {
  repository = "${github_repository.foo.name}"

  configuration {
    url          = "https://www.terraform.io/webhook"
    content_type = "json"
    secret       = "RandomSecretString"
    insecure_ssl = false
  }

  events = ["pull_request"]
}
`, randString)
}

func testAccGithubRepositoryWebhookUpdateConfig(randString string) string {
	return fmt.Sprintf(`
resource "github_repository" "foo" {
  name         = "foo-%s"
  description  = "Terraform acceptance tests"
  homepage_url = "http://example.com/"

  # So that acceptance tests can be run in a github organization
  # with no billing
  private = false

  has_issues    = true
  has_wiki      = true
  has_downloads = true
}

resource "github_repository_webhook" "foo" {
  depends_on = ["github_repository.foo"]
  repository = "foo-%s"

  configuration {
    url          = "https://google.de/webhooks"
    content_type = "form"
    insecure_ssl = false
  }
  active = false

  events = ["issues"]
}
`, randString, randString)
}
