FROM ollama/ollama:latest

# Pre-pull the model during build so first run is instant
# This bakes llama3 into the image — swap for any model you prefer
RUN ollama serve & sleep 5 && ollama pull llama3

EXPOSE 11434

CMD ["ollama", "serve"]