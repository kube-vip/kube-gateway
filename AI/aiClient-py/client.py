from openai import OpenAI

import time 

client = OpenAI(
    api_key='ollama',
    base_url='http://ollama1.ollama:11434/v1',
)



while True:
    time.sleep(8)
    
    chat_completion = client.chat.completions.create(
        messages=[
         {
             'role': 'user',
             'content': 'Say this is a test',
         }
        ],
        model='llama3.2',
    )
    print(chat_completion.choices[0].message.content)
