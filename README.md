# Amazon Price Tracker Telegram Bot

A production-grade, highly cost-optimized, and fully serverless Amazon Product Price Tracker Telegram Bot built in Go.

## Features
- **Add Product**: Simply paste an Amazon link in the chat to start tracking.
- **List Tracked Products**: View all tracked products with current prices and interactive controls.
- **Stop Tracking**: Interactive "❌ Stop Tracking" buttons under products card to remove them.
- **Hourly Price Checks**: Scrapes price changes once every hour.
- **Price Drop Alerts**: Pushes a rich card notification directly to your Telegram chat when a price decrease is detected.
- **Serverless Architecture**: Built on AWS Lambda, API Gateway, and DynamoDB (costs virtually **₹0.00/month** within the AWS Free Tier).

---

## 🛠️ Local Development & Testing

You can run the application entirely on your local machine using **Docker Compose** (for a local DynamoDB instance) and the Go **Local Runner** (uses Telegram long polling).

### Prerequisites
1. [Go 1.22+](https://go.dev/doc/install)
2. [Docker & Docker Compose](https://docs.docker.com/engine/install/)
3. A Telegram Bot Token from [@BotFather](https://t.me/BotFather).

### Setup Steps
1. **Clone & Navigate** to the project directory:
   ```bash
   cd price-tracker
   ```

2. **Start Local DynamoDB**:
   Runs a local DynamoDB database and a web GUI interface.
   ```bash
   docker compose up -d
   ```
   - DynamoDB Local runs at `http://localhost:8000`.
   - DynamoDB Admin GUI runs at [http://localhost:8001](http://localhost:8001) (open in browser to inspect your local tables/records).

3. **Set Environment Variables**:
   Export your Telegram bot token and point the database client to the local DynamoDB endpoint:
   ```bash
   export TELEGRAM_BOT_TOKEN="your-telegram-bot-token"
   export DYNAMODB_ENDPOINT="http://localhost:8000"
   export APP_ENV="local"
   ```

4. **Run Bot Locally**:
   This runs the runner in long-polling mode (it will automatically check and initialize the DynamoDB tables `PriceTrackerUsers` and `PriceTrackerSubscriptions` on the local database):
   ```bash
   go run cmd/local-runner/main.go
   ```

5. **Test Unit Cases**:
   Run unit tests for the URL normalization, ASIN parser, and price parser rules:
   ```bash
   go test -v ./...
   ```

---

## 🚀 AWS Deployment (Serverless)

We use the **AWS Serverless Application Model (SAM)** to deploy the stack to AWS APIGateway, Lambdas, DynamoDB, and EventBridge.

### Manual Deployment via CLI

1. **Install AWS SAM CLI**: Follow the [installation instructions](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-install.html).

2. **Store your Telegram Bot Token in AWS SSM**:
   AWS Systems Manager (SSM) Parameter Store is free. Store your token securely:
   ```bash
   aws ssm put-parameter \
     --name "/price-tracker/telegram-bot-token" \
     --value "YOUR_TELEGRAM_BOT_TOKEN" \
     --type "SecureString" \
     --overwrite
   ```

3. **Build Code**:
   ```bash
   sam build
   ```

4. **Deploy Stack**:
   ```bash
   sam deploy --guided
   ```
   Follow the prompts. Give the stack a name (e.g., `price-tracker`) and select your region (e.g., `ap-south-1`). This will return a `WebhookURL` in the outputs block once successful.

5. **Set the Webhook with Telegram**:
   Configure Telegram to route incoming messages to your API Gateway endpoint:
   ```bash
   curl -F "url=YOUR_DEPLOYED_WEBHOOK_URL" https://api.telegram.org/botYOUR_TELEGRAM_BOT_TOKEN/setWebhook
   ```

---

## 🔄 CI/CD Deployment with GitHub Actions

The repository is configured with two GitHub Actions workflows:

### 1. CI Pipeline (`.github/workflows/ci.yml`)
- Triggered on pull requests and commits to `main`.
- Validates code style (`gofmt`).
- Runs unit tests (`go test`).

### 2. CD Pipeline (`.github/workflows/cd.yml`)
- Triggered on merges to `main`.
- Compiles the Go code and deploys it to your AWS account automatically.

### Setup GitHub Secrets
To allow GitHub to deploy to your AWS account, configure the following secrets in your GitHub repository settings under **Settings > Secrets and variables > Actions**:
1. `AWS_ACCESS_KEY_ID`: Your AWS IAM User Access Key ID.
2. `AWS_SECRET_ACCESS_KEY`: Your AWS IAM User Secret Access Key.

Ensure your IAM user has permissions to deploy CloudFormation stacks, Lambda functions, API Gateway instances, DynamoDB tables, and IAM Roles.
