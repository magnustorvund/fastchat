'use client'

import { useState, useEffect, useRef } from 'react'
import { PaperAirplaneIcon } from '@heroicons/react/24/solid'

interface Message {
  id: number
  username: string
  content: string
  type: 'message' | 'private' | 'system'
  timestamp: Date
}

interface Command {
  name: string
  description: string
}

export default function Chat() {
  const [messages, setMessages] = useState<Message[]>([])
  const [inputMessage, setInputMessage] = useState('')
  const [username, setUsername] = useState('')
  const [ws, setWs] = useState<WebSocket | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const encryptionKeyRef = useRef<Uint8Array | null>(null)
  const [showCommands, setShowCommands] = useState(false)

  useEffect(() => {
    const name = prompt('Enter your username:') || 'Anonymous'
    setUsername(name)

    const websocket = new WebSocket(`ws://localhost:8080/ws?username=${name}`)
    setWs(websocket)

    websocket.onmessage = async (e) => {
      console.log("Received raw message:", e.data);
      
      if (e.data.startsWith('ENCRYPTION_KEY:')) {
        const keyBase64 = e.data.replace('ENCRYPTION_KEY:', '').trim()
        try {
          const binaryStr = window.atob(keyBase64)
          const key = new Uint8Array(binaryStr.length)
          for (let i = 0; i < binaryStr.length; i++) {
            key[i] = binaryStr.charCodeAt(i)
          }
          encryptionKeyRef.current = key
          console.log("Received encryption key, length:", key.length, "bytes")
          return
        } catch (error) {
          console.error("Error setting encryption key:", error)
          return
        }
      }

      if (e.data.includes('FinanceBot 🤖:')) {
        const [botName, ...contentParts] = e.data.split(': ');
        const newMessage: Message = {
          id: Date.now(),
          username: botName,
          content: contentParts.join(': '),
          type: 'message' as const,
          timestamp: new Date()
        };
        console.log("Adding bot message:", newMessage);
        setMessages(prev => [...prev, newMessage]);
        return;
      }

      try {
        // Check if it's a private message
        if (e.data.startsWith('[Private')) {
          const encryptedPart = e.data.split(': ')[1];
          const decryptedContent = await decryptMessage(encryptedPart, encryptionKeyRef.current);
          const newMessage: Message = {
            id: Date.now(),
            username: decryptedContent.includes(':') 
              ? `🔒 ${decryptedContent.split(':')[0]}`
              : `🔒 ${decryptedContent}`,
            content: decryptedContent.includes(':') 
              ? decryptedContent.split(':')[1] 
              : decryptedContent,
            type: 'private',
            timestamp: new Date()
          };
          setMessages(prev => [...prev, newMessage]);
          return;
        }

        // Handle other messages
        const decryptedContent = await decryptMessage(e.data, encryptionKeyRef.current);
        const newMessage: Message = {
          id: Date.now(),
          username: decryptedContent.includes(':') ? decryptedContent.split(':')[0] : decryptedContent,
          content: decryptedContent.includes(':') ? decryptedContent.split(':')[1] : decryptedContent,
          type: decryptedContent.includes('[Private]') ? 'private' : 
                decryptedContent.includes('joined') ? 'system' : 'message',
          timestamp: new Date()
        }

        setMessages(prev => [...prev, newMessage])
      } catch (error) {
        console.error("Error processing message:", error);
      }
    }

    return () => {
      websocket.close()
    }
  }, [])

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const decryptMessage = async (encryptedMsg: string, key: Uint8Array | null) => {
    if (!key) {
      console.log('Waiting for encryption key...')
      return encryptedMsg
    }

    try {
      const cryptoKey = await crypto.subtle.importKey(
        "raw",
        key,
        {
          name: "AES-GCM",
          length: 256
        },
        false,
        ["decrypt"]
      )

      const encryptedData = atob(encryptedMsg)
      const encryptedArray = new Uint8Array(encryptedData.split('').map(c => c.charCodeAt(0)))
      
      const nonce = encryptedArray.slice(0, 12)
      const ciphertext = encryptedArray.slice(12)

      const decrypted = await crypto.subtle.decrypt(
        {
          name: "AES-GCM",
          iv: nonce
        },
        cryptoKey,
        ciphertext
      )

      return new TextDecoder().decode(decrypted)
    } catch (error) {
      console.log('Message might not be encrypted:', encryptedMsg)
      return encryptedMsg
    }
  }

  const sendMessage = (e: React.FormEvent) => {
    e.preventDefault()
    if (inputMessage.trim() && ws) {
      ws.send(inputMessage)
      setInputMessage('')
    }
  }

  const commands: Command[] = [
    {
      name: 'saving',
      description: '💰 Calculate your 10-year savings potential'
    }
  ]

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setInputMessage(value)
    
    // Show commands when typing '/'
    setShowCommands(value.startsWith('/'))
  }

  return (
    <div className="flex flex-col h-screen bg-gray-100">
      <header className="bg-orange-600 text-white p-4 shadow-md">
        <div className="max-w-3xl mx-auto flex items-center">
          <h1 className="text-2xl font-bold">🚀gofastChat</h1>
          <span className="ml-4 bg-orange-500 px-3 py-1 rounded-full text-sm">
            {username}
          </span>
        </div>
      </header>

      <div className="flex-1 p-4 overflow-y-auto">
        <div className="max-w-3xl mx-auto space-y-4">
          {messages.map((msg) => (
            <div
              key={msg.id}
              className={`p-4 rounded-lg ${
                msg.type === 'system' 
                  ? 'bg-gray-200 text-gray-600' 
                  : msg.type === 'private'
                  ? 'bg-orange-100 text-orange-800'
                  : msg.username === username
                  ? 'bg-orange-500 text-white ml-auto'
                  : 'bg-white text-gray-800'
              } ${
                msg.type !== 'system' && 'max-w-[80%]'
              }`}
            >
              {msg.type !== 'system' && (
                <div className="font-semibold text-sm mb-1">
                  {msg.username}
                </div>
              )}
              <div className="break-words">
                {msg.content}
              </div>
              <div className="text-xs opacity-75 mt-1">
                {msg.timestamp.toLocaleTimeString()}
              </div>
            </div>
          ))}
          <div ref={messagesEndRef} />
        </div>
      </div>

      <form onSubmit={sendMessage} className="p-4 border-t bg-white relative">
        <div className="max-w-3xl mx-auto flex space-x-4">
          <div className="flex-1 relative">
            <input
              type="text"
              value={inputMessage}
              onChange={handleInputChange}
              placeholder="Type / for commands..."
              className="w-full p-2 border rounded-lg focus:outline-none focus:ring-2 focus:ring-orange-500 text-black font-medium placeholder-gray-400"
            />
            
            {/* Commands dropdown */}
            {showCommands && (
              <div className="absolute bottom-full mb-2 w-full bg-white rounded-lg shadow-lg border p-2">
                <div className="text-sm text-gray-500 mb-2">Available Commands:</div>
                {commands.map(cmd => (
                  <div
                    key={cmd.name}
                    className="p-2 hover:bg-gray-100 rounded cursor-pointer flex items-center"
                    onClick={() => {
                      setInputMessage(`/${cmd.name}`)
                      setShowCommands(false)
                    }}
                  >
                    <div>
                      <div className="font-medium text-black">/{cmd.name}</div>
                      <div className="text-sm text-gray-500">{cmd.description}</div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          <button
            type="submit"
            className="px-4 py-2 bg-orange-500 text-white rounded-lg hover:bg-orange-600 focus:outline-none focus:ring-2 focus:ring-orange-500 focus:ring-offset-2"
          >
            <PaperAirplaneIcon className="h-5 w-5" />
          </button>
        </div>
      </form>
    </div>
  )
} 